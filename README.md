# k8s-node-proxy

A lightweight proxy server that automatically discovers Kubernetes NodePort services and forwards traffic to current cluster nodes.

![Demo Screenshot](demo_cluster.png)

## The Problem This Solves

In Kubernetes environments, **NodePort services** expose applications on a static port (30000-32767) across all cluster nodes. However, accessing these services presents challenges:

### 🔍 **Dynamic Node IPs**
- **GKE node IPs change** during cluster autoscaling, upgrades, or maintenance
- **Direct node access** requires constantly updating IP addresses in applications
- **LoadBalancer services** work but add cost and complexity for internal tools

### 🎯 **Development & Internal Tools**
- **CI/CD pipelines** need stable endpoints to run tests against NodePort services
- **Internal applications** require predictable URLs for service-to-service communication
- **Development environments** need consistent access points that survive cluster changes

## How k8s-node-proxy Solves This

k8s-node-proxy acts as a **stable gateway** that sits outside your Kubernetes cluster and automatically:

1. **🔍 Discovers NodePort services** in your target namespace via Kubernetes API
2. **📡 Monitors cluster nodes** and their health status continuously
3. **🔄 Routes traffic intelligently** to healthy nodes with automatic failover
4. **⚡ Provides static endpoints** that never change regardless of cluster state

### Architecture Flow
```
External Client → k8s-node-proxy:30001 → healthy-node-ip:30001 → NodePort Service → Pod
```

### Key Benefits
- **🎯 Stable URLs**: Access `proxy-vm:30001` instead of `changing-node-ip:30001`
- **🔄 Automatic failover**: Routes around unhealthy nodes (45-second max failover)
- **📊 Real-time monitoring**: Web UI shows cluster status, node health, and service discovery
- **🛡️ Namespace isolation**: Scoped to specific namespace to avoid port conflicts
- **⚙️ Zero configuration**: Auto-discovers everything through Kubernetes API

## Setup

### Option 1: Manual Setup
1. **Deploy on Google Cloud VM** with proper service account permissions (see Requirements below)
2. **Set environment variables**:
   - `PROJECT_ID=your-gcp-project` or `GOOGLE_CLOUD_PROJECT=your-gcp-project`
   - `NAMESPACE=your-target-namespace`
3. **Run**: `./k8s-node-proxy`

### Option 2: Automated GCP Deployment
See [deployment/gcp/README.md](deployment/gcp/README.md) for VPC-only deployment scripts.

The proxy will:
- Discover all NodePort services in the specified namespace
- Start listeners on those ports
- Forward traffic to current cluster nodes automatically

## Configuration

### Required Environment Variables
- `PROJECT_ID` or `GOOGLE_CLOUD_PROJECT`: GCP project containing the GKE cluster
- `NAMESPACE`: Kubernetes namespace to discover NodePort services from

### Optional Environment Variables
- `PROXY_SERVICE_PORT`: Port for the management interface (default: 80)

## Requirements

### Google Cloud Permissions
The application requires a Google Cloud VM with a service account having the following IAM roles:
- **`roles/container.viewer`**: To discover GKE clusters, access cluster metadata, and list services
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
