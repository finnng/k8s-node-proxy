# GCP Deployment

Scripts to deploy k8s-node-proxy on Google Cloud Platform.

## Prerequisites

- gcloud CLI installed and authenticated
- GCP project with GKE cluster

## Deployment Steps

### 1. Setup Service Account

```bash
PROJECT_ID=my-project ./setup-service-account.sh
```

This script will:
- Create service account `k8s-node-proxy-sa`
- Grant `container.viewer` role
- Enable required APIs (container, compute)

### 2. Deploy VM

```bash
PROJECT_ID=my-project NAMESPACE=production ./deploy-vm.sh
```

This script will:
- Create GCP VM with Container OS running the proxy in Docker
- If VM exists, update the container instead
- Temporarily assign external IP for docker image download, then remove it
- Create necessary firewall rules for the target network
- Use the service account with minimal required permissions
- VM accessible only within VPC network after external IP removal

## Environment Variables

### Required
- `PROJECT_ID` - GCP project ID (environment variable)
- `NAMESPACE` - Kubernetes namespace to discover NodePort services from

### Optional variables for `deploy-vm.sh`:

#### Basic Configuration
- `VM_NAME` - VM instance name (default: k8s-node-proxy)
- `ZONE` - GCP zone (default: us-central1-a)
- `MACHINE_TYPE` - VM size (default: f1-micro)
- `SOURCE_IMAGE` - Docker Hub image (default: finnng/k8s-node-proxy:latest)
- `PROXY_SERVICE_PORT` - Service port (default: 80)

### Network Configuration (Optional)
- `NETWORK` - Custom VPC network name (if not provided, uses default network)
- `SUBNET` - Custom subnet name (required when NETWORK is provided)

**Note**: Firewall rules are automatically created for the selected network (default or custom)

### Examples

**Basic deployment (default network):**
```bash
PROJECT_ID=my-project NAMESPACE=production ./deploy-vm.sh
```

**Using custom Docker image:**
```bash
PROJECT_ID=my-project NAMESPACE=production SOURCE_IMAGE=finnng/k8s-node-proxy:v1.0.0 ./deploy-vm.sh
```

**Using custom VPC network:**
```bash
PROJECT_ID=my-project NAMESPACE=production NETWORK=my-vpc SUBNET=my-subnet ./deploy-vm.sh
```

**Full custom configuration:**
```bash
PROJECT_ID=my-project \
NAMESPACE=production \
SOURCE_IMAGE=finnng/k8s-node-proxy:v1.0.0 \
VM_NAME=proxy \
ZONE=asia-southeast1-a \
NETWORK=staging-network \
SUBNET=subnet-southeast \
./deploy-vm.sh
```

## Network Configuration

### Default Network
When no network configuration is provided, the VM is deployed to the default VPC network.

### Custom Network
When using custom VPC networks, you must provide both `NETWORK` and `SUBNET`. The script will:

1. **Validate** that the specified network and subnet exist
2. **Deploy VM** to the specified network/subnet

**Important**: The script automatically creates the following firewall rules for the target network:
- `k8s-node-proxy-ssh-{network}` - Allows SSH access (port 22)
- `k8s-node-proxy-http-{network}` - Allows HTTP access (port 80) for management interface
- `k8s-node-proxy-nodeports-{network}` - Allows NodePort access (ports 30000-32767)

### Network Requirements
- VM is deployed with **internal IP only** after initial setup (external IP removed after docker image download)
- SSH access requires **IAP tunnel** with `--internal-ip` flag
- Service access only available within the VPC network
- **Firewall rules**: Script automatically creates required firewall rules for the target network

## Cleanup

### 3. Teardown Resources

```bash
PROJECT_ID=my-project ./teardown.sh
```

This script will safely remove all resources created by the deployment scripts:
- VM instance with confirmation prompt
- Firewall rules for the target network
- Service account and IAM bindings

The script is **idempotent** - safe to run multiple times without errors.

**Environment variables (same as deploy-vm.sh):**
- `PROJECT_ID` - GCP project ID (environment variable)
- `VM_NAME` - VM instance name to delete
- `ZONE` - GCP zone where VM is located
- `NETWORK` - Network name for firewall rule cleanup

**Manual cleanup (if needed):**
```bash
# Delete VM
gcloud compute instances delete k8s-node-proxy --zone=us-central1-a

# Delete firewall rules (replace 'default' with your network name if using custom network)
gcloud compute firewall-rules delete \
  k8s-node-proxy-ssh-default \
  k8s-node-proxy-http-default \
  k8s-node-proxy-nodeports-default
```

## Access

VM accessible only within VPC:

1. **From GCP resources**: Access directly via internal IP within same VPC
2. **SSH access**: Use gcloud compute ssh with `--internal-ip` and IAP tunnel

### SSH Commands

**Basic SSH:**
```bash
gcloud compute ssh k8s-node-proxy --zone=us-central1-a --internal-ip --tunnel-through-iap
```

**View logs:**
```bash
gcloud compute ssh k8s-node-proxy --zone=us-central1-a --internal-ip --tunnel-through-iap \
  --command='docker logs $(docker ps -q)'
```