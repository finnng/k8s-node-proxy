# Node Monitoring Feature Implementation Plan

## Overview
Implement dynamic node IP monitoring and selection to ensure node IPs are always accessible. The system will monitor current node health and automatically failover to the oldest available healthy node when needed.

## Current Architecture Analysis
- **Node IP Discovery**: `internal/discovery/k8s.go` handles node IP discovery with basic caching (5min TTL)
- **Node Selection**: Currently selects first available node from first cluster using GCE-specific API
- **Monitoring**: No continuous monitoring - only discovers once at startup
- **Platform Dependency**: Currently GCP-specific using Compute Engine API

## Goals
1. Make the system platform-independent (work on any Kubernetes cluster)
2. Implement reliable node health checking using Kubernetes native APIs
3. Provide automatic failover to oldest healthy node
4. Display node status information on homepage
5. Maintain stable connections by only switching when current node fails

## Implementation Plan

### Phase 1: Platform Independence Migration
**Migrate from GCE-specific to Kubernetes Node API**

#### Current Problem:
```go
// GCE-specific code in internal/discovery/k8s.go
instances, err := d.computeSvc.Instances.List(d.projectID, zone)
```

#### Solution: Use Kubernetes Node API
- Replace GCE instance discovery with `k8sClientset.CoreV1().Nodes().List()`
- Get node addresses from `node.Status.Addresses` (ExternalIP/InternalIP)
- Implement age-based node selection using `CreationTimestamp`

### Phase 2: Enhanced Node Discovery
**Implement comprehensive node discovery with metadata**

#### New Node Structure:
```go
type NodeInfo struct {
    Name         string
    IP           string
    Status       NodeStatus
    Age          time.Duration
    CreationTime time.Time
    LastCheck    time.Time
}
```

#### Key Functions:
- `getAllNodesWithMetadata()` - Get all cluster nodes with full info
- `getNodeExternalOrInternalIP()` - Prefer External IP, fallback to Internal
- `findOldestHealthyNode()` - Select node with longest age and healthy status

### Phase 3: Health Monitoring Service
**Simple reactive monitoring using Kubernetes Node Ready condition**

#### Monitoring Approach:
- **Method**: Use Kubernetes Node API to check `NodeReady` condition
- **Frequency**: Check current node every 60 seconds
- **Failover**: Switch after 3 consecutive health check failures
- **Strategy**: Reactive only - no monitoring of unused nodes

#### Health Check Implementation:
```go
func (m *NodeMonitor) isCurrentNodeHealthy() bool {
    node, err := m.k8sClientset.CoreV1().Nodes().Get(ctx, m.currentNodeName, metav1.GetOptions{})
    if err != nil { return false }

    for _, condition := range node.Status.Conditions {
        if condition.Type == corev1.NodeReady {
            return condition.Status == corev1.ConditionTrue
        }
    }
    return false
}
```

### Phase 4: Node Selection Logic
**Implement oldest-node-first selection with failover**

#### Selection Criteria:
1. **Primary**: Node must be healthy (NodeReady = True)
2. **Secondary**: Prefer oldest node (by CreationTimestamp)
3. **Fallback**: If current node fails, select next oldest healthy node

#### Failover Logic:
- Monitor only current active node
- Switch nodes only on health check failure
- Cache available nodes, refresh on failover only
- No excessive switching - respect node stability

### Phase 5: Display System
**Update homepage to show node status information**

#### Display Components:
- **Current Node**: Name, IP, status, age, last health check
- **Available Nodes Table**: All nodes with status indicators
- **Health Status**: Visual indicators (✅ healthy, ❌ unhealthy, ⚠️ unknown)

#### UI Updates:
- Extend `internal/server/homepage.go`
- Add node status to `ServerInfo` struct
- Real-time status display

### Phase 6: Integration and Testing
**Integrate with existing proxy logic and test thoroughly**

#### Integration Points:
- Maintain existing `GetCurrentNodeIP()` interface
- Update server initialization to start monitoring
- Ensure graceful shutdown of monitoring service

#### Testing Strategy:
- Test node failover scenarios
- Verify health check accuracy
- Test on different Kubernetes platforms (GKE, EKS, AKS)
- Performance testing with monitoring overhead

## Key Benefits

### Platform Independence
✅ **Cross-Platform**: Works on GCP, AWS, Azure, on-premises
✅ **Standard APIs**: Uses only Kubernetes native APIs
✅ **No Cloud Dependencies**: Removes GCE-specific code

### Reliability
✅ **Authoritative Health**: Uses same NodeReady condition as Kubernetes
✅ **Stable Selection**: Prefers oldest (most stable) nodes
✅ **Minimal Disruption**: Only switches on actual failures

### Simplicity
✅ **Reactive Monitoring**: Only checks current node
✅ **Low Overhead**: 60-second check intervals
✅ **Existing Interface**: No changes to proxy logic

## Risk Mitigation

### Potential Issues and Solutions:

1. **Health vs Accessibility Gap**
   - **Risk**: Node healthy in K8s but network unreachable
   - **Solution**: Add network connectivity check as secondary validation

2. **Connection Disruption**
   - **Risk**: Active connections break during node switch
   - **Solution**: Only switch on repeated failures, not single incidents

3. **New Node Discovery**
   - **Risk**: Missing newly added nodes
   - **Solution**: Light periodic refresh (every 10-15 minutes)

## Success Criteria
- [ ] Platform-independent operation on any Kubernetes cluster
- [ ] Automatic failover within 3 minutes of node failure
- [ ] Stable operation without unnecessary node switching
- [ ] Clear visibility into node status and health
- [ ] Zero downtime during healthy node operation

## Implementation Order
1. Migrate GCE to K8s Node API (Platform Independence)
2. Enhanced node discovery with metadata
3. Health monitoring service implementation
4. Node selection and failover logic
5. Homepage display updates
6. Integration and comprehensive testing