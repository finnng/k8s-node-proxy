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

// GetCurrentNodeIP returns the IP address of the currently selected node
func (d *GenericNodeDiscovery) GetCurrentNodeIP(ctx context.Context) (string, error) {
	d.mutex.RLock()
	if d.currentNodeIP != "" && time.Since(d.lastCheck) < 30*time.Second {
		ip := d.currentNodeIP
		d.mutex.RUnlock()
		return ip, nil
	}
	d.mutex.RUnlock()

	// Need to discover/select a node
	return d.discoverNodeIP(ctx)
}

// discoverNodeIP discovers and selects the best node, returning its IP
func (d *GenericNodeDiscovery) discoverNodeIP(ctx context.Context) (string, error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	// Check if we have a cached node that's still valid
	if d.currentNodeIP != "" && time.Since(d.cacheTime) < d.cacheTTL {
		d.lastCheck = time.Now()
		return d.currentNodeIP, nil
	}

	// Get all nodes with metadata
	nodes, err := d.getAllNodesWithMetadata(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get nodes: %w", err)
	}

	if len(nodes) == 0 {
		return "", fmt.Errorf("no nodes found in cluster")
	}

	// Find the oldest healthy node
	selectedNode := d.findOldestHealthyNode(nodes)
	if selectedNode == nil {
		return "", fmt.Errorf("no healthy nodes found")
	}

	// Update current node
	d.currentNodeName = selectedNode.Name
	d.currentNodeIP = selectedNode.IP
	d.cacheTime = time.Now()
	d.lastCheck = time.Now()
	d.failureCount = 0

	slog.Info("Selected node for proxying",
		"node", d.currentNodeName,
		"ip", d.currentNodeIP,
		"age", selectedNode.Age)

	return d.currentNodeIP, nil
}

// getAllNodesWithMetadata retrieves all nodes with their metadata
func (d *GenericNodeDiscovery) getAllNodesWithMetadata(ctx context.Context) ([]NodeInfo, error) {
	// Check cache first
	d.mutex.RLock()
	if len(d.cachedNodes) > 0 && time.Since(d.cacheTime) < d.cacheTTL {
		nodes := make([]NodeInfo, len(d.cachedNodes))
		copy(nodes, d.cachedNodes)
		d.mutex.RUnlock()
		return nodes, nil
	}
	d.mutex.RUnlock()

	// Fetch from API
	nodeList, err := d.k8sClientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	var nodes []NodeInfo
	for _, node := range nodeList.Items {
		nodeInfo := d.nodeToNodeInfo(&node)
		nodes = append(nodes, nodeInfo)
	}

	// Update cache
	d.mutex.Lock()
	d.cachedNodes = make([]NodeInfo, len(nodes))
	copy(d.cachedNodes, nodes)
	d.cacheTime = time.Now()
	d.mutex.Unlock()

	slog.Info("Retrieved nodes from cluster", "count", len(nodes))
	return nodes, nil
}

// nodeToNodeInfo converts a Kubernetes node to NodeInfo
func (d *GenericNodeDiscovery) nodeToNodeInfo(node *corev1.Node) NodeInfo {
	creationTime := node.CreationTimestamp.Time
	age := time.Since(creationTime)

	// Determine status
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

	// Get external IP
	var externalIP string
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeExternalIP {
			externalIP = addr.Address
			break
		}
	}

	// Fall back to internal IP if no external IP
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

// findOldestHealthyNode finds the oldest healthy node from the list
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

	// Sort by creation time (oldest first)
	sort.Slice(healthyNodes, func(i, j int) bool {
		return healthyNodes[i].CreationTime.Before(healthyNodes[j].CreationTime)
	})

	return &healthyNodes[0]
}

// GetAllNodes returns all nodes in the cluster
func (d *GenericNodeDiscovery) GetAllNodes(ctx context.Context) ([]NodeInfo, error) {
	return d.getAllNodesWithMetadata(ctx)
}

// StartHealthMonitoring starts the health monitoring goroutine
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

// StopHealthMonitoring stops the health monitoring
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

// healthMonitorLoop runs the health monitoring loop
func (d *GenericNodeDiscovery) healthMonitorLoop() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-d.monitorCtx.Done():
			return
		case <-ticker.C:
			d.performHealthCheck()
		}
	}
}

// performHealthCheck checks the health of the current node
func (d *GenericNodeDiscovery) performHealthCheck() {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if d.currentNodeName == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get current node status
	node, err := d.k8sClientset.CoreV1().Nodes().Get(ctx, d.currentNodeName, metav1.GetOptions{})
	if err != nil {
		slog.Warn("Failed to get node status", "node", d.currentNodeName, "error", err)
		d.handleNodeFailure()
		return
	}

	// Check if node is still ready
	isHealthy := false
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			isHealthy = condition.Status == corev1.ConditionTrue
			break
		}
	}

	d.updateCurrentNodeLastCheck(d.currentNodeName, time.Now(), isHealthy)

	if !isHealthy {
		slog.Warn("Node became unhealthy", "node", d.currentNodeName)
		d.handleNodeFailure()
	} else {
		// Reset failure count on successful check
		if d.failureCount > 0 {
			slog.Info("Node recovered", "node", d.currentNodeName)
			d.failureCount = 0
		}
	}
}

// updateCurrentNodeLastCheck updates the last check time for the current node
func (d *GenericNodeDiscovery) updateCurrentNodeLastCheck(nodeName string, lastCheck time.Time, isHealthy bool) {
	d.lastCheck = lastCheck
	// Update cached node status if we have it
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

// handleNodeFailure increments failure count and triggers failover if needed
func (d *GenericNodeDiscovery) handleNodeFailure() {
	d.failureCount++
	slog.Warn("Node health check failed",
		"node", d.currentNodeName,
		"failure_count", d.failureCount)

	if d.failureCount >= 3 {
		slog.Error("Node failed 3 consecutive health checks, triggering failover",
			"node", d.currentNodeName)
		d.performFailover()
	}
}

// performFailover selects a new healthy node
func (d *GenericNodeDiscovery) performFailover() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get fresh node list
	nodes, err := d.getAllNodesWithMetadata(ctx)
	if err != nil {
		slog.Error("Failed to get nodes during failover", "error", err)
		return
	}

	// Find a different healthy node
	var candidate *NodeInfo
	for _, node := range nodes {
		if node.Status == NodeHealthy && node.Name != d.currentNodeName {
			if candidate == nil || node.CreationTime.Before(candidate.CreationTime) {
				candidate = &node
			}
		}
	}

	if candidate == nil {
		slog.Error("No healthy replacement nodes found during failover")
		return
	}

	// Switch to new node
	oldNode := d.currentNodeName
	d.currentNodeName = candidate.Name
	d.currentNodeIP = candidate.IP
	d.failureCount = 0
	d.lastCheck = time.Now()

	slog.Info("Failover completed",
		"old_node", oldNode,
		"new_node", d.currentNodeName,
		"new_ip", d.currentNodeIP)
}

// GetCurrentNodeName returns the name of the currently selected node
func (d *GenericNodeDiscovery) GetCurrentNodeName() string {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.currentNodeName
}
