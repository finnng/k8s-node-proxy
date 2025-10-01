#!/bin/bash

# Script to update running k8s-node-proxy container with latest image from Docker Hub
# Temporarily adds external IP to pull image, then removes it
# This script is safe to run multiple times

set -e

# Configuration
PROJECT_ID=${PROJECT_ID}
VM_NAME=${VM_NAME:-"k8s-node-proxy"}
ZONE=${ZONE:-"us-central1-a"}
SOURCE_IMAGE=${SOURCE_IMAGE:-"finnng/k8s-node-proxy:latest"}
PROXY_SERVICE_PORT=${PROXY_SERVICE_PORT:-"80"}
NAMESPACE=${NAMESPACE:-""}

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
  echo "Usage: PROJECT_ID=my-project NAMESPACE=my-namespace $0"
  echo "Environment variables:"
  echo "  PROJECT_ID=${PROJECT_ID} (required)"
  echo "  NAMESPACE=${NAMESPACE} (required)"
  echo "  VM_NAME=${VM_NAME}"
  echo "  ZONE=${ZONE}"
  echo "  SOURCE_IMAGE=${SOURCE_IMAGE}"
  echo "  PROXY_SERVICE_PORT=${PROXY_SERVICE_PORT}"
  exit 1
fi

if [[ -z "$NAMESPACE" ]]; then
  echo "ERROR: NAMESPACE environment variable is required"
  echo "Usage: PROJECT_ID=my-project NAMESPACE=my-namespace $0"
  exit 1
fi

# Display configuration
info "Update Configuration:"
echo "  Project ID: $PROJECT_ID"
echo "  VM Name: $VM_NAME"
echo "  Zone: $ZONE"
echo "  Docker Image: $SOURCE_IMAGE"
echo "  Namespace: $NAMESPACE"
echo "  Service Port: $PROXY_SERVICE_PORT"
echo

# Set project
gcloud config set project "$PROJECT_ID" || error "Failed to set project"

# Check if VM exists
if ! gcloud compute instances describe "$VM_NAME" --zone="$ZONE" &>/dev/null; then
  error "VM $VM_NAME does not exist in zone $ZONE"
fi

# Check if VM is running
VM_STATUS=$(gcloud compute instances describe "$VM_NAME" --zone="$ZONE" --format="get(status)")
if [[ "$VM_STATUS" != "RUNNING" ]]; then
  error "VM is not running (status: $VM_STATUS)"
fi

info "VM found and running"

# Check if external IP already exists
EXTERNAL_IP=$(gcloud compute instances describe "$VM_NAME" \
  --zone="$ZONE" \
  --format="get(networkInterfaces[0].accessConfigs[0].natIP)" 2>/dev/null || echo "")

HAD_EXTERNAL_IP=false
if [[ -n "$EXTERNAL_IP" ]]; then
  info "VM already has external IP: $EXTERNAL_IP"
  HAD_EXTERNAL_IP=true
else
  # Add external IP temporarily
  info "Adding temporary external IP for Docker Hub access..."
  gcloud compute instances add-access-config "$VM_NAME" \
    --zone="$ZONE" \
    --access-config-name="external-nat" || error "Failed to add external IP"

  # Wait for network to stabilize
  info "Waiting for network to stabilize..."
  sleep 10

  # Get the newly assigned external IP
  EXTERNAL_IP=$(gcloud compute instances describe "$VM_NAME" \
    --zone="$ZONE" \
    --format="get(networkInterfaces[0].accessConfigs[0].natIP)")
  info "Temporary external IP assigned: $EXTERNAL_IP"
fi

# Function to cleanup (remove external IP if we added it)
cleanup() {
  if [[ "$HAD_EXTERNAL_IP" == "false" ]]; then
    info "Removing temporary external IP..."
    gcloud compute instances delete-access-config "$VM_NAME" \
      --zone="$ZONE" \
      --access-config-name="external-nat" 2>/dev/null || warn "Failed to remove external IP"
  fi
}

# Set trap to ensure cleanup happens even on error
trap cleanup EXIT

# Build environment variables string (same as deploy-vm.sh)
ENV_VARS_STRING="-e PROJECT_ID=${PROJECT_ID}"
if [[ "$PROXY_SERVICE_PORT" != "80" ]]; then
  ENV_VARS_STRING="${ENV_VARS_STRING} -e PROXY_SERVICE_PORT=${PROXY_SERVICE_PORT}"
fi
if [[ -n "$NAMESPACE" ]]; then
  ENV_VARS_STRING="${ENV_VARS_STRING} -e NAMESPACE=${NAMESPACE}"
fi

# Pull latest image and recreate container
info "Pulling latest image from Docker Hub..."
gcloud compute ssh "$VM_NAME" --project="$PROJECT_ID" --zone="$ZONE" --command="
  set -e
  echo 'Pulling image...'
  sudo docker pull --platform=linux/amd64 $SOURCE_IMAGE || exit 1

  # Check for running container
  CONTAINER_ID=\$(sudo docker ps -q)
  if [[ -n \"\$CONTAINER_ID\" ]]; then
    echo 'Stopping old container...'
    sudo docker stop \$CONTAINER_ID || exit 1
    sudo docker rm \$CONTAINER_ID || exit 1
  else
    echo 'No running container found, checking for stopped containers...'
    STOPPED_CONTAINER=\$(sudo docker ps -a -q)
    if [[ -n \"\$STOPPED_CONTAINER\" ]]; then
      echo 'Removing stopped container...'
      sudo docker rm \$STOPPED_CONTAINER || true
    fi
  fi

  echo 'Starting new container with updated image...'
  sudo docker run -d --restart=always \
    --network host \
    $ENV_VARS_STRING \
    $SOURCE_IMAGE || exit 1
  echo 'Container recreated successfully'
" || error "Failed to pull image and recreate container"

info "Waiting for new container to start..."
sleep 10

info "Waiting for service to be ready..."
sleep 5

# Get internal IP for verification
INTERNAL_IP=$(gcloud compute instances describe "$VM_NAME" \
  --zone="$ZONE" \
  --format="get(networkInterfaces[0].networkIP)")

# Verify service is responding
info "Verifying service health..."
for i in $(seq 1 12); do
  if gcloud compute ssh "$VM_NAME" --project="$PROJECT_ID" --zone="$ZONE" --command="curl -s http://localhost/health" 2>/dev/null | grep -q "OK"; then
    info "Service is healthy"
    break
  fi
  [ $i -eq 12 ] && warn "Service health check timeout - container may still be starting"
  sleep 5
done

# Cleanup will be called automatically by trap

info "Update completed successfully!"
echo
echo "VM Details:"
echo "  Name: $VM_NAME"
echo "  Zone: $ZONE"
echo "  Internal IP: $INTERNAL_IP"
if [[ "$HAD_EXTERNAL_IP" == "true" ]]; then
  echo "  External IP: $EXTERNAL_IP (preserved)"
else
  echo "  External IP: (removed)"
fi
echo
echo "Useful commands:"
echo "  # View container logs:"
echo "  gcloud compute ssh $VM_NAME --project=$PROJECT_ID --zone=$ZONE --internal-ip --tunnel-through-iap --command='sudo docker logs \$(sudo docker ps -q)'"
echo
