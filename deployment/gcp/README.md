# GCP Deployment

Scripts to deploy k8s-node-proxy on Google Cloud Platform.

## Prerequisites

- gcloud CLI installed and authenticated
- GCP project with GKE cluster

## Deployment Steps

### 1. Setup Service Account

```bash
./setup-service-account.sh PROJECT_ID
```

This script will:
- Create service account `k8s-node-proxy-sa`
- Grant `container.clusterViewer` role
- Enable required APIs (container, compute)

### 2. Deploy VM

```bash
./deploy-vm.sh PROJECT_ID
```

This script will:
- Create GCP VM with Container OS running the proxy in Docker
- If VM exists, update the container instead
- Deploy with internal IP only
- Use the service account with minimal required permissions
- VM accessible only within VPC network

## Environment Variables

Optional variables for `deploy-vm.sh`:

- `VM_NAME` - VM instance name (default: k8s-node-proxy)
- `ZONE` - GCP zone (default: us-central1-a)
- `MACHINE_TYPE` - VM size (default: f1-micro)
- `DOCKER_IMAGE` - Container image (default: finnng/k8s-node-proxy:latest)
- `PROXY_SERVICE_PORT` - Service port (default: 80)

Example:
```bash
VM_NAME=my-proxy ZONE=us-west1-a ./deploy-vm.sh my-project
```

## Access

VM accessible only within VPC:

1. **From GCP resources**: Access directly via internal IP within same VPC
2. **SSH access**: Use gcloud compute ssh for management