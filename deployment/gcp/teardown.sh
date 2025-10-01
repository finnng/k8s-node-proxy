#!/bin/bash

# Simplified teardown script for k8s-node-proxy GCP resources
set -e

# Configuration
PROJECT_ID=${PROJECT_ID}
VM_NAME=${VM_NAME:-"k8s-node-proxy"}
ZONE=${ZONE:-"us-central1-a"}
SERVICE_ACCOUNT_NAME="k8s-node-proxy-sa"
# Optional network configuration (if not provided, uses default network)
NETWORK=${NETWORK:-""}

echo() {
    command echo "$@"
}

confirm() {
    read -p "$1 [y/N]: " choice
    [[ "$choice" =~ ^[Yy] ]]
}

# Validate inputs
[[ -z "$PROJECT_ID" ]] && {
    echo "Usage: PROJECT_ID=my-project $0"
    echo "Environment variables:"
    echo "  PROJECT_ID=${PROJECT_ID} (required)"
    echo "  VM_NAME=${VM_NAME}"
    echo "  ZONE=${ZONE}"
    echo "  NETWORK=${NETWORK}"
    echo
    echo "Examples:"
    echo "  PROJECT_ID=my-project $0"
    echo "  PROJECT_ID=my-project VM_NAME=my-proxy ZONE=asia-southeast1-a $0"
    echo "  PROJECT_ID=my-project NETWORK=staging-network $0"
    exit 1
}

# Derived values
SERVICE_ACCOUNT_EMAIL="${SERVICE_ACCOUNT_NAME}@${PROJECT_ID}.iam.gserviceaccount.com"

# Determine target network for firewall rules (matches deploy-vm.sh logic)
TARGET_NETWORK="default"
if [[ -n "$NETWORK" ]]; then
    TARGET_NETWORK="$NETWORK"
fi

# Check dependencies
command -v gcloud >/dev/null || { echo "gcloud CLI not found"; exit 1; }
gcloud config set project "$PROJECT_ID" >/dev/null

# Display all environment variables
echo "Environment Configuration:"
echo "  PROJECT_ID=$PROJECT_ID"
echo "  VM_NAME=$VM_NAME"
echo "  ZONE=$ZONE"
if [[ -n "$NETWORK" ]]; then
    echo "  NETWORK=$NETWORK"
else
    echo "  NETWORK=default"
fi
echo "  TARGET_NETWORK=$TARGET_NETWORK"
echo "  SERVICE_ACCOUNT_NAME=$SERVICE_ACCOUNT_NAME"
echo "  SERVICE_ACCOUNT_EMAIL=$SERVICE_ACCOUNT_EMAIL"
echo

confirm "Delete all k8s-node-proxy resources?" || { echo "Cancelled"; exit 0; }

echo "Deleting resources..."

# VM instance
echo "Deleting VM instance: $VM_NAME (zone: $ZONE)"
gcloud compute instances delete "$VM_NAME" --zone="$ZONE" --quiet || true

# Firewall rules
echo "Deleting firewall rules for network: $TARGET_NETWORK"
for rule in "k8s-node-proxy-ssh-$TARGET_NETWORK" "k8s-node-proxy-http-$TARGET_NETWORK" "k8s-node-proxy-nodeports-$TARGET_NETWORK"; do
    echo "  Deleting firewall rule: $rule"
    gcloud compute firewall-rules delete "$rule" --quiet || true
done

# Service account (this automatically removes IAM bindings)
echo "Deleting service account: $SERVICE_ACCOUNT_EMAIL"
gcloud iam service-accounts delete "$SERVICE_ACCOUNT_EMAIL" --quiet || true

# Local files
rm -f .env-gcp

echo "Teardown completed"