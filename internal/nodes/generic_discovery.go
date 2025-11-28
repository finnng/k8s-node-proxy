package nodes

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// GenericNodeDiscovery implements node discovery for any Kubernetes cluster using kubeconfig
type GenericNodeDiscovery struct {
	k8sClientset *kubernetes.Clientset

	// Node selection and health monitoring
	mutex           sync.RWMutex
	cachedNodes     []NodeInfo
	cacheTime       time.Time
	cacheTTL        time.Duration
	currentNodeName string
	currentNodeIP   string
	failureCount    int
	lastCheck       time.Time

	// Health monitoring
	monitoring bool
	monitorCtx context.Context
	cancel     context.CancelFunc
}

// NewGenericNodeDiscovery creates a new generic Kubernetes node discovery instance
func NewGenericNodeDiscovery(k8sClientset *kubernetes.Clientset) (*GenericNodeDiscovery, error) {
	slog.Info("Initializing Generic Kubernetes node discovery")

	monitorCtx, cancel := context.WithCancel(context.Background())

	return &GenericNodeDiscovery{
		k8sClientset: k8sClientset,
		cacheTTL:     2 * time.Minute, // Same as GKE implementation
		monitorCtx:   monitorCtx,
		cancel:       cancel,
	}, nil
}

func (d *GenericNodeDiscovery) GetCurrentNodeIP(ctx context.Context) (string, error) {
	d.mutex.RLock()
	if d.currentNodeIP != "" && time.Since(d.lastCheck) < 30*time.Second {
		ip := d.currentNodeIP
		d.mutex.RUnlock()
		return ip, nil
	}
	d.mutex.RUnlock()

	return d.discoverNodeIP(ctx)
}

func (d *GenericNodeDiscovery) discoverNodeIP(ctx context.Context) (string, error) {
	d.mutex.Lock()
	if d.currentNodeIP != "" && time.Since(d.cacheTime) < d.cacheTTL {
		d.lastCheck = time.Now()
		d.mutex.Unlock()
		return d.currentNodeIP, nil
	}
	d.mutex.Unlock()

	nodes, err := d.getAllNodesWithMetadata(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get nodes: %w", err)
	}

	if len(nodes) == 0 {
		return "", fmt.Errorf("no nodes found in cluster")
	}

	selectedNode := d.findOldestHealthyNode(nodes)
	if selectedNode == nil {
		return "", fmt.Errorf("no healthy nodes found")
	}

	d.mutex.Lock()
	d.currentNodeName = selectedNode.Name
	d.currentNodeIP = selectedNode.IP
	d.cacheTime = time.Now()
	d.lastCheck = time.Now()
	d.failureCount = 0
	d.mutex.Unlock()

	slog.Info("Selected node for proxying",
		"node", selectedNode.Name,
		"ip", selectedNode.IP,
		"age", selectedNode.Age)

	return selectedNode.IP, nil
}

func (d *GenericNodeDiscovery) getAllNodesWithMetadata(ctx context.Context) ([]NodeInfo, error) {
	d.mutex.RLock()
	if len(d.cachedNodes) > 0 && time.Since(d.cacheTime) < d.cacheTTL {
		nodes := make([]NodeInfo, len(d.cachedNodes))
		copy(nodes, d.cachedNodes)
		d.mutex.RUnlock()
		return nodes, nil
	}
	d.mutex.RUnlock()

	nodeList, err := d.k8sClientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	var nodes []NodeInfo
	for _, node := range nodeList.Items {
		nodeInfo := d.nodeToNodeInfo(&node)
		nodes = append(nodes, nodeInfo)
	}

	d.mutex.Lock()
	d.cachedNodes = make([]NodeInfo, len(nodes))
	copy(d.cachedNodes, nodes)
	d.cacheTime = time.Now()
	d.mutex.Unlock()

	slog.Info("Retrieved nodes from cluster", "count", len(nodes))
	return nodes, nil
}

func (d *GenericNodeDiscovery) nodeToNodeInfo(node *corev1.Node) NodeInfo {
	creationTime := node.CreationTimestamp.Time
	age := time.Since(creationTime)

	status := NodeUnknown
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			if condition.Status == corev1.ConditionTrue {
				status = NodeHealthy
			} else {
				status = NodeUnhealthy
			}
			break
		}
	}

	var externalIP string
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeExternalIP {
			externalIP = addr.Address
			break
		}
	}

	if externalIP == "" {
		for _, addr := range node.Status.Addresses {
			if addr.Type == corev1.NodeInternalIP {
				externalIP = addr.Address
				break
			}
		}
	}

	return NodeInfo{
		Name:         node.Name,
		IP:           externalIP,
		Status:       status,
		Age:          age,
		CreationTime: creationTime,
		LastCheck:    time.Now(),
	}
}

func (d *GenericNodeDiscovery) findOldestHealthyNode(nodes []NodeInfo) *NodeInfo {
	var healthyNodes []NodeInfo
	for _, node := range nodes {
		if node.Status == NodeHealthy {
			healthyNodes = append(healthyNodes, node)
		}
	}

	if len(healthyNodes) == 0 {
		return nil
	}

	sort.Slice(healthyNodes, func(i, j int) bool {
		return healthyNodes[i].CreationTime.Before(healthyNodes[j].CreationTime)
	})

	return &healthyNodes[0]
}

func (d *GenericNodeDiscovery) GetAllNodes(ctx context.Context) ([]NodeInfo, error) {
	return d.getAllNodesWithMetadata(ctx)
}

func (d *GenericNodeDiscovery) StartHealthMonitoring() {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if d.monitoring {
		return
	}

	d.monitoring = true
	go d.healthMonitorLoop()
	slog.Info("Started health monitoring for Generic Kubernetes nodes")
}

func (d *GenericNodeDiscovery) StopHealthMonitoring() {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if !d.monitoring {
		return
	}

	d.monitoring = false
	if d.cancel != nil {
		d.cancel()
	}
	slog.Info("Stopped health monitoring for Generic Kubernetes nodes")
}

func (d *GenericNodeDiscovery) healthMonitorLoop() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	defer slog.Info("Generic health monitoring stopped")

	for {
		select {
		case <-d.monitorCtx.Done():
			slog.Info("Generic health monitoring received stop signal")
			return
		case <-ticker.C:
			d.performHealthCheck()
		}
	}
}

func (d *GenericNodeDiscovery) performHealthCheck() {
	d.mutex.Lock()
	nodeName := d.currentNodeName
	d.mutex.Unlock()

	if nodeName == "" {
		return
	}

	ctx, cancel := context.WithTimeout(d.monitorCtx, 10*time.Second)
	defer cancel()

	node, err := d.k8sClientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		slog.Warn("Failed to get node status", "node", nodeName, "error", err)
		d.handleNodeFailure()
		return
	}

	isHealthy := false
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			isHealthy = condition.Status == corev1.ConditionTrue
			break
		}
	}

	d.mutex.Lock()
	d.updateCurrentNodeLastCheck(nodeName, time.Now(), isHealthy)
	d.mutex.Unlock()

	if !isHealthy {
		slog.Warn("Node became unhealthy", "node", nodeName)
		d.handleNodeFailure()
	} else {
		d.mutex.Lock()
		if d.failureCount > 0 {
			slog.Info("Node recovered", "node", nodeName)
			d.failureCount = 0
		}
		d.mutex.Unlock()
	}
}

func (d *GenericNodeDiscovery) updateCurrentNodeLastCheck(nodeName string, lastCheck time.Time, isHealthy bool) {
	d.lastCheck = lastCheck
	for i := range d.cachedNodes {
		if d.cachedNodes[i].Name == nodeName {
			d.cachedNodes[i].LastCheck = lastCheck
			if isHealthy {
				d.cachedNodes[i].Status = NodeHealthy
			} else {
				d.cachedNodes[i].Status = NodeUnhealthy
			}
			break
		}
	}
}

func (d *GenericNodeDiscovery) handleNodeFailure() {
	d.mutex.Lock()
	d.failureCount++
	nodeName := d.currentNodeName
	shouldFailover := d.failureCount >= 3
	d.mutex.Unlock()

	slog.Warn("Node health check failed",
		"node", nodeName,
		"failure_count", d.failureCount)

	if shouldFailover {
		slog.Error("Node failed 3 consecutive health checks, triggering failover",
			"node", nodeName)
		d.performFailover()
	}
}

func (d *GenericNodeDiscovery) performFailover() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	nodes, err := d.getAllNodesWithMetadata(ctx)
	if err != nil {
		slog.Error("Failed to get nodes during failover", "error", err)
		return
	}

	d.mutex.RLock()
	currentNode := d.currentNodeName
	d.mutex.RUnlock()

	var candidate *NodeInfo
	for _, node := range nodes {
		if node.Status == NodeHealthy && node.Name != currentNode {
			if candidate == nil || node.CreationTime.Before(candidate.CreationTime) {
				candidate = &node
			}
		}
	}

	if candidate == nil {
		slog.Error("No healthy replacement nodes found during failover")
		return
	}

	d.mutex.Lock()
	oldNode := d.currentNodeName
	d.currentNodeName = candidate.Name
	d.currentNodeIP = candidate.IP
	d.failureCount = 0
	d.lastCheck = time.Now()
	d.mutex.Unlock()

	slog.Info("Failover completed",
		"old_node", oldNode,
		"new_node", candidate.Name,
		"new_ip", candidate.IP)
}

func (d *GenericNodeDiscovery) GetCurrentNodeName() string {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.currentNodeName
}
