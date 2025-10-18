#!/bin/bash

# Script to deploy k8s-node-proxy to GCP VM with Docker
# Creates VM with internal IP only - can use default or custom VPC network
# Optionally joins existing network/subnet if NETWORK and SUBNET are provided
# This script is idempotent - safe to run multiple times

set -e

# Configuration
PROJECT_ID=${PROJECT_ID}
VM_NAME=${VM_NAME:-"k8s-node-proxy"}
ZONE=${ZONE:-"us-central1-a"}
MACHINE_TYPE=${MACHINE_TYPE:-"f1-micro"}
SERVICE_ACCOUNT_NAME="k8s-node-proxy-sa"
SOURCE_IMAGE=${SOURCE_IMAGE:-"finnng/k8s-node-proxy:latest"}
PROXY_SERVICE_PORT=${PROXY_SERVICE_PORT:-"80"}
# Optional namespace configuration (if not provided, discovers services in all namespaces)
NAMESPACE=${NAMESPACE:-""}
# Optional network configuration (if not provided, uses default network)
NETWORK=${NETWORK:-""}
SUBNET=${SUBNET:-""}

# Track whether we created a new VM (for cleanup purposes)
VM_CREATED=false

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

# Cleanup function to remove external IP on exit (success or failure)
cleanup() {
  # Only attempt cleanup if we created a new VM in this script run
  if [[ "$VM_CREATED" == "true" ]]; then
    info "Running cleanup: Removing external IP from VM..."
    
    # Check if VM exists and has an external IP
    if gcloud compute instances describe "$VM_NAME" --zone="$ZONE" --format="get(networkInterfaces[0].accessConfigs[0].natIP)" 2>/dev/null | grep -qE '^[0-9]'; then
      if gcloud compute instances delete-access-config "$VM_NAME" \
        --zone="$ZONE" \
        --access-config-name="external-nat" 2>/dev/null; then
        info "External IP removed successfully"
      else
        warn "Failed to remove external IP"
      fi
    else
      info "No external IP found on VM (may have been removed already)"
    fi
  fi
  
  # Exit code is automatically preserved by not using exit/return
}

# Set trap to run cleanup on exit, interruption, and termination
trap cleanup EXIT INT TERM

# Validate required parameters
if [[ -z "$PROJECT_ID" ]]; then
  echo "Usage: PROJECT_ID=my-project NAMESPACE=my-namespace $0"
  echo "Environment variables:"
  echo "  PROJECT_ID=${PROJECT_ID} (required)"
  echo "  NAMESPACE=${NAMESPACE} (required)"
  echo "  VM_NAME=${VM_NAME}"
  echo "  ZONE=${ZONE}"
  echo "  MACHINE_TYPE=${MACHINE_TYPE}"
  echo "  SOURCE_IMAGE=${SOURCE_IMAGE}"
  echo "  PROXY_SERVICE_PORT=${PROXY_SERVICE_PORT}"
  echo "  NETWORK=${NETWORK}"
  echo "  SUBNET=${SUBNET}"
  echo
  echo "Examples:"
  echo "  PROJECT_ID=my-project NAMESPACE=production $0"
  echo "  PROJECT_ID=my-project NAMESPACE=my-namespace VM_NAME=my-proxy ZONE=us-west1-a $0"
  echo "  PROJECT_ID=my-project NAMESPACE=production NETWORK=my-custom-network SUBNET=my-subnet $0"
  exit 1
fi

if [[ -z "$NAMESPACE" ]]; then
  echo "ERROR: NAMESPACE environment variable is required"
  echo "Usage: PROJECT_ID=my-project NAMESPACE=my-namespace $0"
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
echo "  Docker Hub Image: $SOURCE_IMAGE"
echo "  Service Port: $PROXY_SERVICE_PORT"
echo "  Service Account: $SERVICE_ACCOUNT_EMAIL"
echo "  Namespace: $NAMESPACE"
if [[ -n "$NETWORK" ]]; then
  echo "  Network: $NETWORK"
  echo "  Subnet: $SUBNET"
else
  echo "  Network: default"
fi
echo

# Confirmation prompt
read -p "Do you want to proceed with this configuration? (y/N): " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
  info "Deployment cancelled by user"
  exit 0
fi
echo

# Check prerequisites
info "Checking prerequisites..."

# Check if gcloud is installed
if ! command -v gcloud &>/dev/null; then
  error "gcloud CLI is not installed. Please install it first."
fi

# Set project
gcloud config set project "$PROJECT_ID" || error "Failed to set project"

# Check if service account exists
if ! gcloud iam service-accounts describe "$SERVICE_ACCOUNT_EMAIL" &>/dev/null; then
  error "Service account $SERVICE_ACCOUNT_EMAIL does not exist. Run setup-service-account.sh first."
fi

# Note: We will pull the image directly from Docker Hub on the VM
info "Will pull image directly from Docker Hub on VM: $SOURCE_IMAGE"

# Determine target network for firewall rules
TARGET_NETWORK="default"
if [[ -n "$NETWORK" ]]; then
  info "Validating custom network configuration..."

  # Check if network exists
  if ! gcloud compute networks describe "$NETWORK" &>/dev/null; then
    error "Network $NETWORK does not exist. Please create it first or use existing network."
  fi

  # Check if subnet exists (required when using custom network)
  if [[ -z "$SUBNET" ]]; then
    error "SUBNET must be provided when using custom NETWORK"
  fi

  if ! gcloud compute networks subnets describe "$SUBNET" --region="${ZONE%-*}" &>/dev/null; then
    error "Subnet $SUBNET does not exist in region ${ZONE%-*}. Please create it first or use existing subnet."
  fi

  TARGET_NETWORK="$NETWORK"
  info "Custom network validation completed"
else
  info "Using default network"
fi

# Create firewall rules for the target network
info "Creating firewall rules for network: $TARGET_NETWORK"

# Create SSH firewall rule
SSH_RULE_NAME="k8s-node-proxy-ssh-${TARGET_NETWORK}"
if ! gcloud compute firewall-rules describe "$SSH_RULE_NAME" &>/dev/null; then
  info "Creating SSH firewall rule: $SSH_RULE_NAME"
  gcloud compute firewall-rules create "$SSH_RULE_NAME" \
    --allow tcp:22 \
    --source-ranges 0.0.0.0/0 \
    --target-tags k8s-node-proxy-ssh \
    --network "$TARGET_NETWORK" \
    --description "Allow SSH access for k8s-node-proxy VMs" || warn "Failed to create SSH firewall rule (may already exist)"
else
  info "SSH firewall rule already exists: $SSH_RULE_NAME"
fi

# Create HTTP firewall rule
HTTP_RULE_NAME="k8s-node-proxy-http-${TARGET_NETWORK}"
if ! gcloud compute firewall-rules describe "$HTTP_RULE_NAME" &>/dev/null; then
  info "Creating HTTP firewall rule: $HTTP_RULE_NAME"
  gcloud compute firewall-rules create "$HTTP_RULE_NAME" \
    --allow tcp:80 \
    --source-ranges 0.0.0.0/0 \
    --target-tags k8s-node-proxy-http \
    --network "$TARGET_NETWORK" \
    --description "Allow HTTP access for k8s-node-proxy management interface" || warn "Failed to create HTTP firewall rule (may already exist)"
else
  info "HTTP firewall rule already exists: $HTTP_RULE_NAME"
fi

# Create NodePort range firewall rule
NODEPORT_RULE_NAME="k8s-node-proxy-nodeports-${TARGET_NETWORK}"
if ! gcloud compute firewall-rules describe "$NODEPORT_RULE_NAME" &>/dev/null; then
  info "Creating NodePort firewall rule: $NODEPORT_RULE_NAME"
  gcloud compute firewall-rules create "$NODEPORT_RULE_NAME" \
    --allow tcp:30000-32767 \
    --source-ranges 0.0.0.0/0 \
    --target-tags k8s-node-proxy-nodeports \
    --network "$TARGET_NETWORK" \
    --description "Allow NodePort access for k8s-node-proxy" || warn "Failed to create NodePort firewall rule (may already exist)"
else
  info "NodePort firewall rule already exists: $NODEPORT_RULE_NAME"
fi

# Create environment variables for the container
ENV_VARS="PROJECT_ID=${PROJECT_ID}"
if [[ "$PROXY_SERVICE_PORT" != "80" ]]; then
  ENV_VARS="${ENV_VARS},PROXY_SERVICE_PORT=${PROXY_SERVICE_PORT}"
fi
if [[ -n "$NAMESPACE" ]]; then
  ENV_VARS="${ENV_VARS},NAMESPACE=${NAMESPACE}"
fi

# Check if VM already exists
if gcloud compute instances describe "$VM_NAME" --zone="$ZONE" &>/dev/null; then
  info "VM already exists, updating container..."
  gcloud compute instances update-container "$VM_NAME" \
    --zone="$ZONE" \
    --container-image="$SOURCE_IMAGE" \
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

# Create VM with Container OS
info "Creating VM with Container OS..."

# Build gcloud command with optional network parameters
if [[ -n "$NETWORK" ]]; then
  info "Creating VM with custom network: $NETWORK, subnet: $SUBNET"
  gcloud compute instances create-with-container "$VM_NAME" \
    --zone="$ZONE" \
    --machine-type="$MACHINE_TYPE" \
    --container-image="$SOURCE_IMAGE" \
    --container-env="$ENV_VARS" \
    --service-account="$SERVICE_ACCOUNT_EMAIL" \
    --scopes=cloud-platform \
    --can-ip-forward \
    --tags=k8s-node-proxy-ssh,k8s-node-proxy-http,k8s-node-proxy-nodeports \
    --boot-disk-size=10GB \
    --boot-disk-type=pd-standard \
    --image-family=cos-stable \
    --image-project=cos-cloud \
    --network="$NETWORK" \
    --subnet="$SUBNET" || error "Failed to create VM"
else
  info "Creating VM with default network"
  gcloud compute instances create-with-container "$VM_NAME" \
    --zone="$ZONE" \
    --machine-type="$MACHINE_TYPE" \
    --container-image="$SOURCE_IMAGE" \
    --container-env="$ENV_VARS" \
    --service-account="$SERVICE_ACCOUNT_EMAIL" \
    --scopes=cloud-platform \
    --can-ip-forward \
    --tags=k8s-node-proxy-ssh,k8s-node-proxy-http,k8s-node-proxy-nodeports \
    --boot-disk-size=10GB \
    --boot-disk-type=pd-standard \
    --image-family=cos-stable \
    --image-project=cos-cloud || error "Failed to create VM"
fi

# Mark that we created a new VM (for cleanup purposes)
VM_CREATED=true
info "VM created successfully, external IP will be removed after setup"

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

# Wait for SSH to be ready
info "Waiting for SSH to be ready..."
for i in $(seq 1 12); do
  if gcloud compute ssh "$VM_NAME" --project="$PROJECT_ID" --zone="$ZONE" --command="echo 'SSH ready'" &>/dev/null; then
    info "SSH is ready"
    break
  fi
  [ $i -eq 12 ] && error "SSH failed to become ready after 2 minutes"
  sleep 10
done

# Pull Docker Hub image on VM (using external IP while it's available)
info "Pulling Docker Hub image on VM..."
gcloud compute ssh "$VM_NAME" --project="$PROJECT_ID" --zone="$ZONE" --command="
    sudo docker pull --platform=linux/amd64 $SOURCE_IMAGE
" || error "Failed to pull image from Docker Hub"

# Wait for service to be ready
info "Waiting for service to be ready..."
for i in $(seq 1 15); do
  if gcloud compute ssh "$VM_NAME" --project="$PROJECT_ID" --zone="$ZONE" --command="curl -s http://localhost:$PROXY_SERVICE_PORT/health" 2>/dev/null | grep "OK"; then
    info "Service is ready"
    break
  fi
  [ $i -eq 15 ] && warn "Service may still be starting"
  sleep 10
done

# External IP will be removed by cleanup trap
info "Setup completed successfully!"

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
if [[ -n "$NETWORK" ]]; then
  echo "Network: $NETWORK"
  echo "Subnet: $SUBNET"
else
  echo "Network: default"
fi
echo
echo "Useful commands:"
echo "  # SSH to VM (via IAP tunnel):"
echo "  gcloud compute ssh $VM_NAME --project=$PROJECT_ID --zone=$ZONE --internal-ip --tunnel-through-iap"
echo
echo "  # View container logs:"
echo "  gcloud compute ssh $VM_NAME --project=$PROJECT_ID --zone=$ZONE --internal-ip --tunnel-through-iap --command='docker logs \$(docker ps -q)'"
echo
echo "  # Update container:"
echo "  ./deploy-vm.sh $PROJECT_ID"
echo
echo "  # Delete VM:"
echo "  gcloud compute instances delete $VM_NAME --zone=$ZONE"
echo
echo "  # Delete firewall rules:"
echo "  gcloud compute firewall-rules delete k8s-node-proxy-ssh-$TARGET_NETWORK k8s-node-proxy-http-$TARGET_NETWORK k8s-node-proxy-nodeports-$TARGET_NETWORK"
