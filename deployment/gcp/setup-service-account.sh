#!/bin/bash

# Script to create service account for k8s-node-proxy deployment
# This script is idempotent - safe to run multiple times

set -e

# Configuration
PROJECT_ID="$1"
SERVICE_ACCOUNT_NAME="k8s-node-proxy-sa"
SERVICE_ACCOUNT_EMAIL="${SERVICE_ACCOUNT_NAME}@${PROJECT_ID}.iam.gserviceaccount.com"
DISPLAY_NAME="k8s-node-proxy Service Account"
DESCRIPTION="Service account for k8s-node-proxy to access GKE cluster information"

# Required roles for k8s-node-proxy
REQUIRED_ROLES=(
    "roles/container.clusterViewer"
    "roles/compute.viewer"
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
    error "PROJECT_ID is required. Usage: $0 <PROJECT_ID>"
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

# Create key file (optional - only if doesn't exist)
KEY_FILE="$SERVICE_ACCOUNT_NAME-key.json"
if [[ ! -f "$KEY_FILE" ]]; then
    info "Creating service account key file: $KEY_FILE"
    gcloud iam service-accounts keys create "$KEY_FILE" \
        --iam-account="$SERVICE_ACCOUNT_EMAIL" || error "Failed to create key file"

    warn "IMPORTANT: Store $KEY_FILE securely and never commit it to version control!"
    echo "export GOOGLE_APPLICATION_CREDENTIALS=$(pwd)/$KEY_FILE" > .env-gcp
    info "Environment file created: .env-gcp"
else
    warn "Key file $KEY_FILE already exists, skipping key creation"
fi

info "Service account setup completed successfully!"
echo
echo "Service Account Email: $SERVICE_ACCOUNT_EMAIL"
echo "Key File: $KEY_FILE"
echo
echo "To use this service account, run:"
echo "  source .env-gcp"
echo "  gcloud auth activate-service-account --key-file=$KEY_FILE"