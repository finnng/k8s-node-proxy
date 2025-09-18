#!/bin/bash

# Script to deploy k8s-node-proxy to GCP VM with Docker
# Creates VM with internal IP only - VPC access only
# This script is idempotent - safe to run multiple times

set -e

# Configuration
PROJECT_ID="$1"
VM_NAME=${VM_NAME:-"k8s-node-proxy"}
ZONE=${ZONE:-"us-central1-a"}
MACHINE_TYPE=${MACHINE_TYPE:-"f1-micro"}
SERVICE_ACCOUNT_NAME="k8s-node-proxy-sa"
DOCKER_IMAGE=${DOCKER_IMAGE:-"finnng/k8s-node-proxy:latest"}
PROXY_SERVICE_PORT=${PROXY_SERVICE_PORT:-"80"}

error() {
    echo "ERROR: $1" >&2
    exit 1
}

info() {
    echo "INFO: $1"
}

warn() {
    echo "WARN: $1"
}

# Validate required parameters
if [[ -z "$PROJECT_ID" ]]; then
    echo "Usage: $0 PROJECT_ID"
    echo "Environment variables:"
    echo "  VM_NAME=${VM_NAME}"
    echo "  ZONE=${ZONE}"
    echo "  MACHINE_TYPE=${MACHINE_TYPE}"
    echo "  DOCKER_IMAGE=${DOCKER_IMAGE}"
    echo "  PROXY_SERVICE_PORT=${PROXY_SERVICE_PORT}"
    echo
    echo "Examples:"
    echo "  $0 my-project"
    echo "  VM_NAME=my-proxy ZONE=us-west1-a $0 my-project"
    exit 1
fi

# Derived values
SERVICE_ACCOUNT_EMAIL="${SERVICE_ACCOUNT_NAME}@${PROJECT_ID}.iam.gserviceaccount.com"

# Display configuration
info "Deployment Configuration:"
echo "  Project ID: $PROJECT_ID"
echo "  VM Name: $VM_NAME"
echo "  Zone: $ZONE"
echo "  Machine Type: $MACHINE_TYPE"
echo "  Docker Image: $DOCKER_IMAGE"
echo "  Service Port: $PROXY_SERVICE_PORT"
echo "  Service Account: $SERVICE_ACCOUNT_EMAIL"
echo

# Check prerequisites
info "Checking prerequisites..."

# Check if gcloud is installed
if ! command -v gcloud &> /dev/null; then
    error "gcloud CLI is not installed. Please install it first."
fi

# Set project
gcloud config set project "$PROJECT_ID" || error "Failed to set project"

# Check if service account exists
if ! gcloud iam service-accounts describe "$SERVICE_ACCOUNT_EMAIL" &>/dev/null; then
    error "Service account $SERVICE_ACCOUNT_EMAIL does not exist. Run setup-service-account.sh first."
fi

# Note: VM only has internal IP - VPC access only

# Create environment variables for the container
ENV_VARS="PROJECT_ID=${PROJECT_ID}"
if [[ "$PROXY_SERVICE_PORT" != "80" ]]; then
    ENV_VARS="${ENV_VARS},PROXY_SERVICE_PORT=${PROXY_SERVICE_PORT}"
fi

# Check if VM already exists
if gcloud compute instances describe "$VM_NAME" --zone="$ZONE" &>/dev/null; then
    info "VM already exists, updating container..."
    gcloud compute instances update-container "$VM_NAME" \
        --zone="$ZONE" \
        --container-image="$DOCKER_IMAGE" \
        --container-env="$ENV_VARS" || error "Failed to update container"

    # Get internal IP
    INTERNAL_IP=$(gcloud compute instances describe "$VM_NAME" \
        --zone="$ZONE" \
        --format="get(networkInterfaces[0].networkIP)")

    info "VM updated successfully!"
    echo
    echo "Internal IP: $INTERNAL_IP"
    echo "Service URL: http://$INTERNAL_IP:$PROXY_SERVICE_PORT"
    exit 0
fi

# Create VM with container
info "Creating VM with container..."
gcloud compute instances create-with-container "$VM_NAME" \
    --zone="$ZONE" \
    --machine-type="$MACHINE_TYPE" \
    --container-image="$DOCKER_IMAGE" \
    --container-env="$ENV_VARS" \
    --service-account="$SERVICE_ACCOUNT_EMAIL" \
    --scopes="cloud-platform" \
    --tags="k8s-node-proxy" \
    --boot-disk-size="10GB" \
    --boot-disk-type="pd-standard" \
    --image-family="cos-stable" \
    --image-project="cos-cloud" \
    --no-address || error "Failed to create VM"

# Wait for VM to be ready
info "Waiting for VM to start..."
for i in $(seq 1 30); do
    if gcloud compute instances describe "$VM_NAME" --zone="$ZONE" --format="get(status)" 2>/dev/null | grep -q "RUNNING"; then
        info "VM is running"
        break
    fi
    [ $i -eq 30 ] && error "VM failed to start after 5 minutes"
    sleep 10
done

# Wait for service to be ready
info "Waiting for service to be ready..."
for i in $(seq 1 15); do
    if gcloud compute ssh "$VM_NAME" --zone="$ZONE" --command="curl -s http://localhost:$PROXY_SERVICE_PORT/health" 2>/dev/null | grep -q "OK"; then
        info "Service is ready"
        break
    fi
    [ $i -eq 15 ] && warn "Service may still be starting"
    sleep 10
done

# Get internal IP
INTERNAL_IP=$(gcloud compute instances describe "$VM_NAME" \
    --zone="$ZONE" \
    --format="get(networkInterfaces[0].networkIP)")

# Display deployment info
info "Deployment completed successfully!"
echo
echo "VM Details:"
echo "  Name: $VM_NAME"
echo "  Zone: $ZONE"
echo "  Internal IP: $INTERNAL_IP"
echo "  Service URL: http://$INTERNAL_IP:$PROXY_SERVICE_PORT"
echo
echo "Note: VM accessible only from within VPC"
echo
echo "Useful commands:"
echo "  # SSH to VM:"
echo "  gcloud compute ssh $VM_NAME --zone=$ZONE"
echo
echo "  # View container logs:"
echo "  gcloud compute ssh $VM_NAME --zone=$ZONE --command='docker logs \$(docker ps -q)'"
echo
echo "  # Update container:"
echo "  ./deploy-vm.sh $PROJECT_ID"
echo
echo "  # Delete VM:"
echo "  gcloud compute instances delete $VM_NAME --zone=$ZONE"