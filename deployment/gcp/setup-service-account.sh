#!/bin/bash

# Script to create service account for k8s-node-proxy deployment
# This script is idempotent - safe to run multiple times

set -e

# Configuration
PROJECT_ID=${PROJECT_ID}
SERVICE_ACCOUNT_NAME="k8s-node-proxy-sa"
SERVICE_ACCOUNT_EMAIL="${SERVICE_ACCOUNT_NAME}@${PROJECT_ID}.iam.gserviceaccount.com"
DISPLAY_NAME="k8s-node-proxy Service Account"
DESCRIPTION="Service account for k8s-node-proxy to access GKE cluster information"

# Required roles for k8s-node-proxy
REQUIRED_ROLES=(
    "roles/container.viewer"
)

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

# Validate inputs
if [[ -z "$PROJECT_ID" ]]; then
    error "PROJECT_ID is required. Usage: PROJECT_ID=my-project $0"
fi

# Check if gcloud is installed and authenticated
if ! command -v gcloud &> /dev/null; then
    error "gcloud CLI is not installed. Please install it first."
fi

# Set the project
info "Setting project to: $PROJECT_ID"
gcloud config set project "$PROJECT_ID" || error "Failed to set project"

# Enable required APIs
info "Enabling required APIs..."
gcloud services enable container.googleapis.com compute.googleapis.com || error "Failed to enable APIs"

# Check if service account already exists
if gcloud iam service-accounts describe "$SERVICE_ACCOUNT_EMAIL" &>/dev/null; then
    warn "Service account $SERVICE_ACCOUNT_EMAIL already exists"
else
    info "Creating service account: $SERVICE_ACCOUNT_EMAIL"
    gcloud iam service-accounts create "$SERVICE_ACCOUNT_NAME" \
        --display-name="$DISPLAY_NAME" \
        --description="$DESCRIPTION" || error "Failed to create service account"
    info "Service account created successfully"
fi

# Assign required roles
info "Assigning required roles..."
for role in "${REQUIRED_ROLES[@]}"; do
    info "  Assigning role: $role"
    gcloud projects add-iam-policy-binding "$PROJECT_ID" \
        --member="serviceAccount:$SERVICE_ACCOUNT_EMAIL" \
        --role="$role" &>/dev/null || warn "Failed to assign role $role (may already exist)"
done

info "Service account setup completed successfully!"
echo
echo "Service Account Email: $SERVICE_ACCOUNT_EMAIL"
echo
echo "To use this service account on a VM, ensure the VM has the service account attached:"
echo "  gcloud compute instances create <instance-name> --service-account=$SERVICE_ACCOUNT_EMAIL --scopes=cloud-platform"