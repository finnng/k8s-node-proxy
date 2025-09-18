# k8s-node-proxy

A lightweight proxy server that automatically discovers Kubernetes NodePort services and forwards traffic to current cluster nodes.

## How it works

1. **Auto-discovery**: Connects to GKE clusters using Google Cloud APIs to discover active NodePort services
2. **Dynamic forwarding**: Automatically finds current node IPs and forwards requests transparently
3. **Static access**: Provides stable endpoints for accessing dynamic Kubernetes services

```
Developer → proxy:30001 → current-node-ip:30001 → pod
```

## Use Case

Solves the problem of changing Kubernetes node IPs by providing a stable proxy that automatically tracks node changes and forwards traffic to the right destination.

## Setup

### Option 1: Manual Setup
1. **Deploy on Google Cloud VM** with proper service account permissions (see Requirements below)
2. **Set environment variable**: `PROJECT_ID=your-gcp-project` or `GOOGLE_CLOUD_PROJECT=your-gcp-project`
3. **Run**: `./k8s-node-proxy`

### Option 2: Automated GCP Deployment
See [deployment/gcp/README.md](deployment/gcp/README.md) for VPC-only deployment scripts.

The proxy will:
- Discover all NodePort services in your GKE cluster
- Start listeners on those ports
- Forward traffic to current cluster nodes automatically

## Configuration

### Required Environment Variables
- `PROJECT_ID` or `GOOGLE_CLOUD_PROJECT`: GCP project containing the GKE cluster

### Optional Environment Variables
- `PROXY_SERVICE_PORT`: Port for the management interface (default: 80)

## Requirements

### Google Cloud Permissions
The application requires a Google Cloud VM with a service account having the following IAM roles:
- **`roles/container.clusterViewer`**: To discover GKE clusters and access cluster metadata
- **Network access**: VM must be able to reach the GKE cluster API server

### Runtime Dependencies
- **GKE cluster**: Must be in the same GCP project as the VM
- **Google Application Default Credentials (ADC)**: Automatically available on GCP VMs
- **Go 1.24+**: Only required for building from source

### Network Requirements
- VM must have network connectivity to the GKE cluster API server
- Outbound HTTPS access to Google Cloud APIs (container.googleapis.com)

## License

MIT License

Copyright (c) 2025 k8s-node-proxy contributors

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
