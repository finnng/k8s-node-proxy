package nodes

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"
	"sync"
	"time"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/container/v1"
	"google.golang.org/api/option"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type NodeStatus int

const (
	NodeHealthy NodeStatus = iota
	NodeUnhealthy
	NodeUnknown
)

type NodeInfo struct {
	Name         string
	IP           string
	Status       NodeStatus
	Age          time.Duration
	CreationTime time.Time
	LastCheck    time.Time
}

type NodeDiscovery struct {
	projectID       string
	containerSvc    *container.Service
	k8sClientset    *kubernetes.Clientset
	cachedIP        string
	cachedNodes     []NodeInfo
	currentNodeName string
	cacheTime       time.Time
	cacheTTL        time.Duration
	mutex           sync.RWMutex

	// Health monitoring
	failureCount     int
	failureThreshold int
	checkInterval    time.Duration
	ctx              context.Context
	cancel           context.CancelFunc
}

func New(projectID string) (*NodeDiscovery, error) {
	ctx := context.Background()

	containerSvc, err := container.NewService(ctx, option.WithScopes(container.CloudPlatformScope))
	if err != nil {
		return nil, fmt.Errorf("failed to create container service: %w", err)
	}

	// Build Kubernetes config
	config, _, err := buildK8sConfig(ctx, containerSvc, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to build K8s config: %w", err)
	}

	k8sClientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create K8s clientset: %w", err)
	}

	monitorCtx, cancel := context.WithCancel(context.Background())

	return &NodeDiscovery{
		projectID:        projectID,
		containerSvc:     containerSvc,
		k8sClientset:     k8sClientset,
		cacheTTL:         2 * time.Minute,
		failureThreshold: 3,
		checkInterval:    15 * time.Second,
		ctx:              monitorCtx,
		cancel:           cancel,
	}, nil
}

func (d *NodeDiscovery) GetCurrentNodeIP(ctx context.Context) (string, error) {
	d.mutex.RLock()
	if d.cachedIP != "" && time.Since(d.cacheTime) < d.cacheTTL {
		ip := d.cachedIP
		d.mutex.RUnlock()
		return ip, nil
	}
	d.mutex.RUnlock()

	d.mutex.Lock()
	defer d.mutex.Unlock()

	if d.cachedIP != "" && time.Since(d.cacheTime) < d.cacheTTL {
		return d.cachedIP, nil
	}

	ip, err := d.discoverNodeIP(ctx)
	if err != nil {
		return "", err
	}

	d.cachedIP = ip
	d.cacheTime = time.Now()
	return ip, nil
}

func (d *NodeDiscovery) discoverNodeIP(ctx context.Context) (string, error) {
	// Get all nodes with metadata
	nodeInfos, err := d.getAllNodesWithMetadata(ctx)
	if err != nil {
		return "", err
	}

	if len(nodeInfos) == 0 {
		return "", fmt.Errorf("no nodes found in cluster")
	}

	// Cache the node list for other operations
	d.cachedNodes = nodeInfos

	// Find oldest healthy node
	oldestNode := d.findOldestHealthyNode(nodeInfos)
	if oldestNode == nil {
		// If no healthy nodes, fall back to oldest node regardless of status
		oldestNode = &nodeInfos[0]
	}

	// Set current node name for health monitoring
	d.currentNodeName = oldestNode.Name

	return oldestNode.IP, nil
}

// getAllNodesWithMetadata retrieves all cluster nodes with complete metadata
func (d *NodeDiscovery) getAllNodesWithMetadata(ctx context.Context) ([]NodeInfo, error) {
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

	// Sort by age (oldest first)
	sort.Slice(nodeInfos, func(i, j int) bool {
		return nodeInfos[i].CreationTime.Before(nodeInfos[j].CreationTime)
	})

	return nodeInfos, nil
}

// getNodeStatus determines the health status from node conditions
func getNodeStatus(node corev1.Node) NodeStatus {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			if condition.Status == corev1.ConditionTrue {
				return NodeHealthy
			}
			return NodeUnhealthy
		}
	}
	return NodeUnknown
}

// findOldestHealthyNode selects the oldest node that is healthy
func (d *NodeDiscovery) findOldestHealthyNode(nodes []NodeInfo) *NodeInfo {
	for i := range nodes {
		if nodes[i].Status == NodeHealthy {
			return &nodes[i]
		}
	}
	return nil
}

// GetAllNodes returns cached node information
func (d *NodeDiscovery) GetAllNodes(ctx context.Context) ([]NodeInfo, error) {
	d.mutex.RLock()
	if len(d.cachedNodes) > 0 && time.Since(d.cacheTime) < d.cacheTTL {
		nodes := make([]NodeInfo, len(d.cachedNodes))
		copy(nodes, d.cachedNodes)
		d.mutex.RUnlock()
		return nodes, nil
	}
	d.mutex.RUnlock()

	// Refresh cache if stale
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

// getNodeInternalIP extracts the Internal IP (matching original GCE NetworkIP behavior)
func getNodeInternalIP(node corev1.Node) string {
	// Get Internal IP (equivalent to GCE NetworkIP)
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			return addr.Address
		}
	}

	return ""
}

// findOldestNode returns the node with the earliest creation timestamp
func findOldestNode(nodes []corev1.Node) corev1.Node {
	if len(nodes) == 0 {
		return corev1.Node{}
	}

	// Sort nodes by creation time (oldest first)
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].CreationTimestamp.Before(&nodes[j].CreationTimestamp)
	})

	return nodes[0]
}

// buildK8sConfig creates Kubernetes client configuration
func buildK8sConfig(ctx context.Context, containerSvc *container.Service, projectID string) (*rest.Config, interface{}, error) {
	// Get the first cluster in the project
	clusters, err := containerSvc.Projects.Locations.Clusters.List(
		fmt.Sprintf("projects/%s/locations/-", projectID)).Context(ctx).Do()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list clusters: %w", err)
	}

	if len(clusters.Clusters) == 0 {
		return nil, nil, fmt.Errorf("no clusters found in project %s", projectID)
	}

	cluster := clusters.Clusters[0]

	// Use private endpoint for internal VPC connectivity
	if cluster.PrivateClusterConfig == nil || cluster.PrivateClusterConfig.PrivateEndpoint == "" {
		return nil, nil, fmt.Errorf("cluster %s does not have a private endpoint configured", cluster.Name)
	}
	endpoint := cluster.PrivateClusterConfig.PrivateEndpoint
	fmt.Printf("Using private cluster endpoint: %s\n", endpoint)

	// Decode cluster CA certificate
	caCert, err := base64.StdEncoding.DecodeString(cluster.MasterAuth.ClusterCaCertificate)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode cluster CA certificate: %w", err)
	}

	// Get Google default token source (uses ADC)
	tokenSource, err := google.DefaultTokenSource(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get default token source: %w", err)
	}

	// Get a token to use for authentication
	token, err := tokenSource.Token()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get token: %w", err)
	}

	// Build Kubernetes config
	config := &rest.Config{
		Host: "https://" + endpoint,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: caCert,
		},
		BearerToken: token.AccessToken,
	}

	return config, nil, nil
}

// StartHealthMonitoring begins monitoring the current node's health
func (d *NodeDiscovery) StartHealthMonitoring() {
	go d.healthMonitorLoop()
}

// StopHealthMonitoring gracefully stops the health monitoring
func (d *NodeDiscovery) StopHealthMonitoring() {
	if d.cancel != nil {
		d.cancel()
	}
}

// healthMonitorLoop runs the periodic health check
func (d *NodeDiscovery) healthMonitorLoop() {
	ticker := time.NewTicker(d.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.performHealthCheck()
		case <-d.ctx.Done():
			return
		}
	}
}

// performHealthCheck checks current node health and handles failover
func (d *NodeDiscovery) performHealthCheck() {
	d.mutex.RLock()
	currentNodeName := d.currentNodeName
	d.mutex.RUnlock()

	if currentNodeName == "" {
		// No current node set, skip health check
		return
	}

	now := time.Now()
	isHealthy := d.isCurrentNodeHealthy(currentNodeName)

	// Update the LastCheck time for current node in cached list
	d.updateCurrentNodeLastCheck(currentNodeName, now, isHealthy)

	if isHealthy {
		// Reset failure count on successful check
		d.mutex.Lock()
		d.failureCount = 0
		d.mutex.Unlock()
	} else {
		d.handleNodeFailure()
	}
}

// updateCurrentNodeLastCheck updates the LastCheck time for the current node
func (d *NodeDiscovery) updateCurrentNodeLastCheck(nodeName string, lastCheck time.Time, isHealthy bool) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

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

// isCurrentNodeHealthy checks if the current node is healthy using Kubernetes API
func (d *NodeDiscovery) isCurrentNodeHealthy(nodeName string) bool {
	node, err := d.k8sClientset.CoreV1().Nodes().Get(d.ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		fmt.Printf("Failed to get node %s: %v\n", nodeName, err)
		return false
	}

	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

// handleNodeFailure manages failure counting and triggers failover if needed
func (d *NodeDiscovery) handleNodeFailure() {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	d.failureCount++
	fmt.Printf("Node health check failed (%d/%d)\n", d.failureCount, d.failureThreshold)

	if d.failureCount >= d.failureThreshold {
		fmt.Printf("Node %s failed %d consecutive health checks, initiating failover\n",
			d.currentNodeName, d.failureThreshold)
		d.performFailover()
		d.failureCount = 0 // Reset after failover
	}
}

// performFailover switches to the next best available node
func (d *NodeDiscovery) performFailover() {
	// Clear cache to force fresh discovery
	d.cachedIP = ""
	d.cacheTime = time.Time{}

	// Get fresh node list
	nodes, err := d.getAllNodesWithMetadata(d.ctx)
	if err != nil {
		fmt.Printf("Failed to get nodes for failover: %v\n", err)
		return
	}

	// Find next best node (oldest healthy, excluding current failed node)
	for _, node := range nodes {
		if node.Name != d.currentNodeName && node.Status == NodeHealthy {
			d.cachedIP = node.IP
			d.currentNodeName = node.Name
			d.cacheTime = time.Now()
			fmt.Printf("Failover completed: switched to node %s (%s)\n", node.Name, node.IP)
			return
		}
	}

	fmt.Printf("Warning: No healthy nodes found for failover\n")
}

// GetCurrentNodeName returns the name of the currently selected node
func (d *NodeDiscovery) GetCurrentNodeName() string {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.currentNodeName
}
