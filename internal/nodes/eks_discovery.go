package nodes

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// EKSNodeDiscovery implements node discovery for AWS EKS clusters
type EKSNodeDiscovery struct {
	region       string
	clusterName  string
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

// NewEKSNodeDiscovery creates a new EKS node discovery instance
func NewEKSNodeDiscovery(region, clusterName string, k8sClientset *kubernetes.Clientset) (*EKSNodeDiscovery, error) {
	slog.Info("Initializing EKS node discovery", "region", region, "cluster", clusterName)

	monitorCtx, cancel := context.WithCancel(context.Background())

	return &EKSNodeDiscovery{
		region:       region,
		clusterName:  clusterName,
		k8sClientset: k8sClientset,
		cacheTTL:     2 * time.Minute, // Same as GKE implementation
		monitorCtx:   monitorCtx,
		cancel:       cancel,
	}, nil
}

// GetCurrentNodeIP returns the IP address of the currently selected node
func (d *EKSNodeDiscovery) GetCurrentNodeIP(ctx context.Context) (string, error) {
	d.mutex.RLock()
	if d.currentNodeIP != "" && time.Since(d.lastCheck) < 30*time.Second {
		ip := d.currentNodeIP
		d.mutex.RUnlock()
		return ip, nil
	}
	d.mutex.RUnlock()

	// Refresh current node selection
	return d.discoverNodeIP(ctx)
}

// discoverNodeIP discovers and selects the best node
func (d *EKSNodeDiscovery) discoverNodeIP(ctx context.Context) (string, error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	// Get all nodes
	nodes, err := d.getAllNodesWithMetadata(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get nodes: %w", err)
	}

	if len(nodes) == 0 {
		return "", fmt.Errorf("no nodes found in cluster")
	}

	// Find oldest healthy node
	selectedNode := d.findOldestHealthyNode(nodes)
	if selectedNode == nil {
		return "", fmt.Errorf("no healthy nodes found")
	}

	// Update current selection
	d.currentNodeName = selectedNode.Name
	d.currentNodeIP = selectedNode.IP
	d.lastCheck = time.Now()
	d.failureCount = 0

	slog.Info("Selected EKS node", "node", selectedNode.Name, "ip", selectedNode.IP)
	return d.currentNodeIP, nil
}

// getAllNodesWithMetadata retrieves all nodes with their metadata
func (d *EKSNodeDiscovery) getAllNodesWithMetadata(ctx context.Context) ([]NodeInfo, error) {
	nodes, err := d.k8sClientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	var nodeInfos []NodeInfo
	now := time.Now()

	for _, node := range nodes.Items {
		nodeIP := getNodeInternalIP(node)
		if nodeIP == "" {
			continue // Skip nodes without internal IP
		}

		// Determine node status from conditions
		status := getNodeStatus(node)

		nodeInfo := NodeInfo{
			Name:         node.Name,
			IP:           nodeIP,
			Status:       status,
			Age:          now.Sub(node.CreationTimestamp.Time),
			CreationTime: node.CreationTimestamp.Time,
			LastCheck:    now,
		}
		nodeInfos = append(nodeInfos, nodeInfo)
	}

	// Sort by age (oldest first) - same logic as GKE
	sort.Slice(nodeInfos, func(i, j int) bool {
		return nodeInfos[i].CreationTime.Before(nodeInfos[j].CreationTime)
	})

	return nodeInfos, nil
}

// findOldestHealthyNode selects the oldest node that is healthy
func (d *EKSNodeDiscovery) findOldestHealthyNode(nodes []NodeInfo) *NodeInfo {
	for i := range nodes {
		if nodes[i].Status == NodeHealthy {
			return &nodes[i]
		}
	}
	return nil
}

// GetAllNodes returns cached node information
func (d *EKSNodeDiscovery) GetAllNodes(ctx context.Context) ([]NodeInfo, error) {
	d.mutex.RLock()
	if len(d.cachedNodes) > 0 && time.Since(d.cacheTime) < d.cacheTTL {
		nodes := make([]NodeInfo, len(d.cachedNodes))
		copy(nodes, d.cachedNodes)
		d.mutex.RUnlock()
		return nodes, nil
	}
	d.mutex.RUnlock()

	// Refresh cache
	nodes, err := d.getAllNodesWithMetadata(ctx)
	if err != nil {
		return nil, err
	}

	d.mutex.Lock()
	d.cachedNodes = nodes
	d.cacheTime = time.Now()
	d.mutex.Unlock()

	return nodes, nil
}

// StartHealthMonitoring starts the health monitoring goroutine
func (d *EKSNodeDiscovery) StartHealthMonitoring() {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if d.monitoring {
		return
	}

	d.monitoring = true
	go d.healthMonitorLoop()
	slog.Info("Started EKS node health monitoring")
}

// StopHealthMonitoring stops the health monitoring
func (d *EKSNodeDiscovery) StopHealthMonitoring() {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if !d.monitoring {
		return
	}

	d.monitoring = false
	d.cancel()
	slog.Info("Stopped EKS node health monitoring")
}

// healthMonitorLoop runs the health monitoring loop
func (d *EKSNodeDiscovery) healthMonitorLoop() {
	ticker := time.NewTicker(15 * time.Second) // Same interval as GKE
	defer ticker.Stop()
	defer slog.Info("EKS health monitoring stopped")

	for {
		select {
		case <-d.monitorCtx.Done():
			slog.Info("EKS health monitoring received stop signal")
			return
		case <-ticker.C:
			d.performHealthCheck()
		}
	}
}

// performHealthCheck checks the health of the current node
func (d *EKSNodeDiscovery) performHealthCheck() {
	// Use monitoring context with timeout to respect shutdown signals
	ctx, cancel := context.WithTimeout(d.monitorCtx, 10*time.Second)
	defer cancel()

	d.mutex.RLock()
	nodeName := d.currentNodeName
	d.mutex.RUnlock()

	if nodeName == "" {
		return
	}

	// Check node health via Kubernetes API
	node, err := d.k8sClientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		slog.Warn("Failed to get node for health check", "node", nodeName, "error", err)
		d.handleNodeFailure()
		return
	}

	// Check if node is ready
	status := getNodeStatus(*node)
	d.updateCurrentNodeLastCheck(nodeName, time.Now(), status == NodeHealthy)

	if status != NodeHealthy {
		slog.Warn("Node health check failed", "node", nodeName, "status", status)
		d.handleNodeFailure()
	} else {
		// Reset failure count on success
		d.mutex.Lock()
		d.failureCount = 0
		d.mutex.Unlock()
	}
}

// updateCurrentNodeLastCheck updates the last check time for a node
func (d *EKSNodeDiscovery) updateCurrentNodeLastCheck(nodeName string, lastCheck time.Time, isHealthy bool) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	// Update cached node info
	for i := range d.cachedNodes {
		if d.cachedNodes[i].Name == nodeName {
			d.cachedNodes[i].LastCheck = lastCheck
			if !isHealthy {
				d.cachedNodes[i].Status = NodeUnhealthy
			}
			break
		}
	}
}

// handleNodeFailure handles node failure detection
func (d *EKSNodeDiscovery) handleNodeFailure() {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	d.failureCount++
	slog.Warn("Node failure detected", "node", d.currentNodeName, "failures", d.failureCount)

	if d.failureCount >= 3 { // Same threshold as GKE
		slog.Error("Node has failed 3 health checks, triggering failover", "node", d.currentNodeName)
		d.performFailover()
	}
}

// performFailover selects a new healthy node
func (d *EKSNodeDiscovery) performFailover() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get fresh node list
	nodes, err := d.getAllNodesWithMetadata(ctx)
	if err != nil {
		slog.Error("Failed to get nodes during failover", "error", err)
		return
	}

	// Find new healthy node (excluding current failed node)
	var candidates []NodeInfo
	for _, node := range nodes {
		if node.Name != d.currentNodeName && node.Status == NodeHealthy {
			candidates = append(candidates, node)
		}
	}

	if len(candidates) == 0 {
		slog.Error("No healthy candidate nodes found for failover")
		return
	}

	// Select oldest healthy candidate
	selectedNode := d.findOldestHealthyNode(candidates)
	if selectedNode == nil {
		slog.Error("No healthy nodes available for failover")
		return
	}

	// Update selection
	oldNode := d.currentNodeName
	d.currentNodeName = selectedNode.Name
	d.currentNodeIP = selectedNode.IP
	d.failureCount = 0
	d.lastCheck = time.Now()

	slog.Info("Failover completed", "old_node", oldNode, "new_node", selectedNode.Name, "new_ip", selectedNode.IP)
}

// GetCurrentNodeName returns the name of the currently selected node
func (d *EKSNodeDiscovery) GetCurrentNodeName() string {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.currentNodeName
}
