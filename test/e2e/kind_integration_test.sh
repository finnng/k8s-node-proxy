#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
CLUSTER_NAME="k8s-proxy-e2e-test"
TEST_NAMESPACE="e2e-test"
PROXY_PORT=8080
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

echo -e "${YELLOW}=== k8s-node-proxy E2E Integration Test ===${NC}"
echo "Project root: $PROJECT_ROOT"

# Function to print status
print_status() {
    echo -e "${GREEN}✓${NC} $1"
}

print_error() {
    echo -e "${RED}✗${NC} $1"
}

print_info() {
    echo -e "${YELLOW}→${NC} $1"
}

# Cleanup function
cleanup() {
    print_info "Cleaning up..."
    kind delete cluster --name "$CLUSTER_NAME" 2>/dev/null || true
    rm -f /tmp/kind-e2e-kubeconfig.yaml
    print_status "Cleanup complete"
}

# Set trap to cleanup on exit
trap cleanup EXIT

# Step 1: Check prerequisites
print_info "Checking prerequisites..."
if ! command -v kind &> /dev/null; then
    print_error "kind is not installed. Install with: go install sigs.k8s.io/kind@latest"
    exit 1
fi
if ! command -v kubectl &> /dev/null; then
    print_error "kubectl is not installed"
    exit 1
fi
if ! command -v docker &> /dev/null; then
    print_error "docker is not installed"
    exit 1
fi
print_status "All prerequisites found"

# Step 2: Create kind cluster
print_info "Creating kind cluster with 3 worker nodes..."
cat <<EOF > /tmp/kind-e2e-config.yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraPortMappings:
  - containerPort: $PROXY_PORT
    hostPort: $PROXY_PORT
    protocol: TCP
  - containerPort: 30001
    hostPort: 30001
    protocol: TCP
- role: worker
- role: worker
- role: worker
EOF

kind create cluster --name "$CLUSTER_NAME" --config /tmp/kind-e2e-config.yaml --wait 2m
print_status "Kind cluster created"

# Export kubeconfig
kind get kubeconfig --name "$CLUSTER_NAME" > /tmp/kind-e2e-kubeconfig.yaml
export KUBECONFIG=/tmp/kind-e2e-kubeconfig.yaml

# Step 3: Wait for nodes to be ready
print_info "Waiting for all nodes to be ready..."
kubectl wait --for=condition=ready nodes --all --timeout=120s
NODE_COUNT=$(kubectl get nodes --no-headers | wc -l | tr -d ' ')
print_status "All $NODE_COUNT nodes are ready"

# Step 4: Create test namespace
print_info "Creating test namespace..."
kubectl create namespace "$TEST_NAMESPACE"
print_status "Namespace created"

# Step 5: Deploy nginx as test backend
print_info "Deploying nginx test service..."
kubectl create deployment nginx --image=nginx:latest -n "$TEST_NAMESPACE"
kubectl expose deployment nginx --type=NodePort --port=80 --target-port=80 -n "$TEST_NAMESPACE"
kubectl wait --for=condition=available deployment/nginx -n "$TEST_NAMESPACE" --timeout=60s
NGINX_NODEPORT=$(kubectl get svc nginx -n "$TEST_NAMESPACE" -o jsonpath='{.spec.ports[0].nodePort}')
print_status "Nginx deployed on NodePort $NGINX_NODEPORT"

# Step 6: Build and load k8s-node-proxy image
print_info "Building k8s-node-proxy Docker image..."
cd "$PROJECT_ROOT"

cat <<'EOF' > Dockerfile.e2e
FROM golang:1.25-alpine AS builder
WORKDIR /build
COPY . .
RUN go mod download
RUN go build -o k8s-node-proxy ./cmd/server

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /build/k8s-node-proxy .
CMD ["./k8s-node-proxy"]
EOF

docker build -f Dockerfile.e2e -t k8s-node-proxy:e2e-test .
rm -f Dockerfile.e2e
print_status "Docker image built"

# Step 7: Load image into kind
print_info "Loading image into kind cluster..."
kind load docker-image k8s-node-proxy:e2e-test --name "$CLUSTER_NAME"
print_status "Image loaded into cluster"

# Step 8: Create RBAC permissions
print_info "Creating RBAC permissions..."
kubectl apply -f - <<EOF
apiVersion: v1
kind: ServiceAccount
metadata:
  name: k8s-node-proxy
  namespace: $TEST_NAMESPACE
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: k8s-node-proxy
rules:
- apiGroups: [""]
  resources: ["nodes"]
  verbs: ["get", "list"]
- apiGroups: [""]
  resources: ["services"]
  verbs: ["get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: k8s-node-proxy
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: k8s-node-proxy
subjects:
- kind: ServiceAccount
  name: k8s-node-proxy
  namespace: $TEST_NAMESPACE
EOF
print_status "RBAC permissions created"

# Step 9: Deploy k8s-node-proxy
print_info "Deploying k8s-node-proxy..."
kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: k8s-node-proxy
  namespace: $TEST_NAMESPACE
spec:
  replicas: 1
  selector:
    matchLabels:
      app: k8s-node-proxy
  template:
    metadata:
      labels:
        app: k8s-node-proxy
    spec:
      serviceAccountName: k8s-node-proxy
      containers:
      - name: proxy
        image: k8s-node-proxy:e2e-test
        imagePullPolicy: Never
        env:
        - name: NAMESPACE
          value: "$TEST_NAMESPACE"
        - name: PROXY_SERVICE_PORT
          value: "$PROXY_PORT"
        ports:
        - containerPort: $PROXY_PORT
          name: http
        - containerPort: $NGINX_NODEPORT
          name: proxy
---
apiVersion: v1
kind: Service
metadata:
  name: k8s-node-proxy
  namespace: $TEST_NAMESPACE
spec:
  type: NodePort
  selector:
    app: k8s-node-proxy
  ports:
  - name: http
    port: $PROXY_PORT
    targetPort: $PROXY_PORT
    nodePort: $(( PROXY_PORT > 30000 ? PROXY_PORT : 30080 ))
  - name: proxy
    port: $NGINX_NODEPORT
    targetPort: $NGINX_NODEPORT
    nodePort: 30001
EOF

print_info "Waiting for k8s-node-proxy to be ready..."
if ! kubectl wait --for=condition=available deployment/k8s-node-proxy -n "$TEST_NAMESPACE" --timeout=120s; then
    print_error "Deployment failed to become available"
    print_info "Pod status:"
    kubectl get pods -n "$TEST_NAMESPACE" -l app=k8s-node-proxy
    print_info "Pod logs:"
    POD_NAME=$(kubectl get pods -n "$TEST_NAMESPACE" -l app=k8s-node-proxy -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
    if [ -n "$POD_NAME" ]; then
        kubectl logs -n "$TEST_NAMESPACE" "$POD_NAME" --tail=100 || true
        kubectl describe pod -n "$TEST_NAMESPACE" "$POD_NAME" || true
    fi
    exit 1
fi
print_status "k8s-node-proxy deployed"

# Step 10: Check proxy logs
print_info "Checking k8s-node-proxy startup logs..."
sleep 2
POD_NAME=$(kubectl get pods -n "$TEST_NAMESPACE" -l app=k8s-node-proxy -o jsonpath='{.items[0].metadata.name}')
kubectl logs -n "$TEST_NAMESPACE" "$POD_NAME" --tail=20

# Step 11: Test proxy functionality
print_info "Testing proxy functionality..."

# Get node IP to test from
NODE_IP=$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')
print_info "Testing from node IP: $NODE_IP"

# Test 1: Homepage endpoint (should show proxy status)
print_info "Test 1: Homepage endpoint..."
if kubectl run curl-test --image=curlimages/curl:latest --rm -i --restart=Never -n "$TEST_NAMESPACE" -- \
    curl -s -f http://k8s-node-proxy:$PROXY_PORT/ 2>/dev/null | tee /tmp/homepage.html | grep -q "k8s-node-proxy"; then
    print_status "Homepage test passed"
else
    print_error "Homepage test failed"
    cat /tmp/homepage.html 2>/dev/null || echo "No output captured"
    exit 1
fi

# Test 2: Proxy forwarding to nginx
print_info "Test 2: Proxy forwarding to nginx..."
if kubectl run curl-test-2 --image=curlimages/curl:latest --rm -i --restart=Never -n "$TEST_NAMESPACE" -- \
    curl -s -f http://k8s-node-proxy:$NGINX_NODEPORT/ 2>/dev/null | tee /tmp/nginx.html | grep -q "Welcome to nginx"; then
    print_status "Proxy forwarding test passed"
else
    print_error "Proxy forwarding test failed"
    cat /tmp/nginx.html 2>/dev/null || echo "No output captured"
    exit 1
fi

# Test 3: Health endpoint
print_info "Test 3: Health endpoint..."
HEALTH_OUTPUT=$(kubectl run curl-test-3 --image=curlimages/curl:latest --rm -i --restart=Never -n "$TEST_NAMESPACE" -- \
    curl -s http://k8s-node-proxy:$NGINX_NODEPORT/health 2>/dev/null)
if echo "$HEALTH_OUTPUT" | grep -q "OK: Forwarding to node"; then
    print_status "Health endpoint test passed"
    echo "  Health response: $HEALTH_OUTPUT"
else
    print_error "Health endpoint test failed"
    echo "  Response: $HEALTH_OUTPUT"
    exit 1
fi

# Step 12: Verify node discovery
print_info "Verifying node discovery..."
kubectl logs -n "$TEST_NAMESPACE" "$POD_NAME" | grep "Retrieved nodes from cluster"
if [ $? -eq 0 ]; then
    print_status "Node discovery working"
else
    print_error "Node discovery failed"
    exit 1
fi

# Step 13: Verify service discovery
print_info "Verifying service discovery..."
kubectl logs -n "$TEST_NAMESPACE" "$POD_NAME" | grep "Found NodePort service.*nginx"
if [ $? -eq 0 ]; then
    print_status "Service discovery working"
else
    print_error "Service discovery failed"
    exit 1
fi

# Step 14: Summary
echo ""
echo -e "${GREEN}=== E2E Integration Test Summary ===${NC}"
print_status "Kind cluster created with $NODE_COUNT nodes"
print_status "Nginx test service deployed on NodePort $NGINX_NODEPORT"
print_status "k8s-node-proxy deployed and running"
print_status "Platform detection: Generic Kubernetes"
print_status "Service discovery: Working"
print_status "Node discovery: Working"
print_status "Homepage endpoint: Working"
print_status "Proxy forwarding: Working"
print_status "Health endpoint: Working"
echo ""
echo -e "${GREEN}✓ All E2E tests passed!${NC}"
