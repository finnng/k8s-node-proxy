package main

import (
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s-node-proxy/internal/nodes"
	"k8s-node-proxy/internal/proxy"
	"k8s-node-proxy/internal/services"
)

const eksHomepageTemplate = `
<!DOCTYPE html>
<html>
<head>
    <title>k8s-node-proxy - EKS</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        table { border-collapse: collapse; width: 100%; margin: 20px 0; }
        th, td { border: 1px solid #ddd; padding: 12px; text-align: left; }
        th { background-color: #f2f2f2; }
        .section { margin: 30px 0; }
        h1 { color: #333; }
        h2 { color: #666; }
        .status-healthy { background-color: #d4edda; padding: 4px 8px; border-radius: 4px; }
        .status-unhealthy { background-color: #f8d7da; padding: 4px 8px; border-radius: 4px; }
        .status-unknown { background-color: #fff3cd; padding: 4px 8px; border-radius: 4px; }
        .info-text { font-size: 12px; color: #666; font-style: italic; margin-top: 8px; line-height: 1.4; }
    </style>
</head>
<body>
    <h1>k8s-node-proxy Server (Amazon EKS)</h1>

    <div class="section">
        <h2>Cluster Information</h2>
        <table>
            <tr><th>Property</th><th>Value</th></tr>
            <tr><td>AWS Region</td><td>{{.AWSRegion}}</td></tr>
            <tr><td>Cluster Name</td><td>{{.ClusterName}}</td></tr>
            <tr><td>Kubernetes Endpoint</td><td>{{.K8sEndpoint}}</td></tr>
            <tr><td>Target Namespace</td><td>{{.Namespace}}</td></tr>
        </table>
    </div>

    <div class="section">
        <h2>Current Active Node</h2>
        {{if .CurrentNode}}
        <table>
            <tr><th>Property</th><th>Value</th></tr>
            <tr><td>Node Name</td><td>{{.CurrentNode.Name}}</td></tr>
            <tr><td>IP Address</td><td>{{.CurrentNode.IP}}</td></tr>
            <tr><td>Status</td><td>{{.CurrentNode.Status}}</td></tr>
        </table>
        {{else}}
        <p>No current node selected</p>
        {{end}}
        <div class="info-text">
            Node behavior: Health checks every 15 seconds. Failover after 3 consecutive failures to oldest healthy node (max 45 seconds).
            Node list refreshes every 2 minutes for display only - active node remains stable unless unhealthy.
        </div>
    </div>

    <div class="section">
        <h2>All Cluster Nodes</h2>
        <table>
            <tr><th>Node Name</th><th>IP Address</th><th>Status</th><th>Age</th><th>Last Check</th></tr>
            {{range .AllNodes}}
            <tr>
                <td>{{.Name}}</td>
                <td>{{.IP}}</td>
                <td>
                    {{if eq .Status 0}}<span class="status-healthy">Healthy</span>{{else if eq .Status 1}}<span class="status-unhealthy">Unhealthy</span>{{else}}<span class="status-unknown">Unknown</span>{{end}}
                </td>
                <td>{{printf "%.0f" .Age.Hours}}h</td>
                <td>{{.LastCheck.Format "15:04:05"}}</td>
            </tr>
            {{end}}
        </table>
    </div>

    <div class="section">
        <h2>NodePort Services ({{.Namespace}} namespace)</h2>
        <table>
            <tr><th>Service</th><th>Namespace</th><th>NodePort</th><th>TargetPort</th><th>Protocol</th></tr>
            {{range .Services}}
            <tr>
                <td>{{.Name}}</td>
                <td>{{.Namespace}}</td>
                <td>{{.NodePort}}</td>
                <td>{{.TargetPort}}</td>
                <td>{{.Protocol}}</td>
            </tr>
            {{end}}
        </table>
    </div>

    <div class="section">
        <p><strong>Proxy Status:</strong> Active and forwarding traffic to current cluster nodes</p>
        <p><strong>Health Check:</strong> <a href="/health">/health</a></p>
    </div>
</body>
</html>
`

// EKSServerInfo contains information about the EKS server and cluster
type EKSServerInfo struct {
	AWSRegion   string
	ClusterName string
	K8sEndpoint string
	Namespace   string
	NodeIPs     []string
	Services    []services.ServiceInfo
	CurrentNode *CurrentNodeInfo
	AllNodes    []nodes.NodeInfo
}

// EKSServer is a server implementation for EKS clusters
type EKSServer struct {
	awsRegion       string
	clusterName     string
	servicePort     int
	portManager     *PortManager
	nodeDiscovery   *services.EKSNodePortDiscovery
	nodeIPDiscovery *nodes.EKSNodeDiscovery
	serverInfo      *EKSServerInfo
}

// NewEKSServer creates a new EKS server
func NewEKSServer(awsRegion, clusterName string, servicePort int) (*EKSServer, error) {
	slog.Info("Initializing k8s-node-proxy server for EKS",
		"region", awsRegion,
		"cluster", clusterName,
		"service_port", servicePort)

	nodePortDiscovery, err := services.NewEKSNodePortDiscovery(awsRegion, clusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to create EKS service discovery: %w", err)
	}

	// Create node discovery with the same region, cluster, and clientset
	nodeIPDiscovery, err := nodes.NewEKSNodeDiscovery(awsRegion, clusterName, nodePortDiscovery.GetClientset())
	if err != nil {
		return nil, fmt.Errorf("failed to create EKS node discovery: %w", err)
	}

	server := &EKSServer{
		awsRegion:       awsRegion,
		clusterName:     clusterName,
		servicePort:     servicePort,
		nodeDiscovery:   nodePortDiscovery,
		nodeIPDiscovery: nodeIPDiscovery,
		serverInfo:      nil, // Will be populated during Run()
	}

	// Create port manager
	portManager := NewPortManager()
	server.portManager = portManager

	slog.Info("EKS server initialization completed successfully")
	return server, nil
}

func (s *EKSServer) Run() error {
	ctx := context.Background()

	// Collect server info
	if err := s.collectServerInfo(ctx); err != nil {
		return fmt.Errorf("failed to collect server info: %w", err)
	}

	// Create handlers
	serviceHandler := s.createServiceHandler()
	proxyHandler := proxy.NewHandler(s.nodeIPDiscovery)

	// Start the configured service port for homepage
	if err := s.portManager.StartPort(s.servicePort, serviceHandler); err != nil {
		slog.Error("Failed to start homepage service port", "port", s.servicePort, "error", err)
	}

	// Start health monitoring for node IP discovery
	s.nodeIPDiscovery.StartHealthMonitoring()
	slog.Info("Started node health monitoring")

	// Discover NodePorts once at startup
	ports, err := s.nodeDiscovery.DiscoverNodePorts(ctx)
	if err != nil {
		return err
	}

	// Start proxy ports for discovered services
	for _, port := range ports {
		if err := s.portManager.StartPort(port, proxyHandler); err != nil {
			slog.Error("Failed to start proxy port", "port", port, "error", err)
		}
	}

	// Set up graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	slog.Info("k8s-node-proxy server started successfully for EKS", "service_port", s.servicePort)

	<-c
	slog.Info("Shutting down server...")

	// Stop health monitoring
	s.nodeIPDiscovery.StopHealthMonitoring()

	// Stop all ports
	s.portManager.StopAll()

	slog.Info("Server shutdown complete")
	return nil
}

func (s *EKSServer) collectServerInfo(ctx context.Context) error {
	slog.Info("Collecting EKS server information")

	clusterInfo := s.nodeDiscovery.GetClusterInfo()

	// Get services info
	srvcs, err := s.nodeDiscovery.DiscoverServices(ctx)
	if err != nil {
		return fmt.Errorf("failed to discover services: %w", err)
	}

	// Get node information
	allNodes, err := s.nodeIPDiscovery.GetAllNodes(ctx)
	if err != nil {
		return fmt.Errorf("failed to get all nodes: %w", err)
	}

	// Extract node IPs
	var nodeIPs []string
	for _, node := range allNodes {
		nodeIPs = append(nodeIPs, node.IP)
	}

	s.serverInfo = &EKSServerInfo{
		AWSRegion:   s.awsRegion,
		ClusterName: clusterInfo.Name,
		K8sEndpoint: clusterInfo.Endpoint,
		Namespace:   os.Getenv("NAMESPACE"),
		NodeIPs:     nodeIPs,
		Services:    srvcs,
		AllNodes:    allNodes,
	}

	return nil
}

func (s *EKSServer) createServiceHandler() http.Handler {
	mux := http.NewServeMux()

	// Homepage
	mux.HandleFunc("/", s.handleHomepage)

	// Info endpoint
	mux.HandleFunc("/info", s.handleInfo)

	// Health endpoint
	mux.HandleFunc("/health", s.handleHealth)

	return mux
}

func (s *EKSServer) handleHomepage(w http.ResponseWriter, r *http.Request) {
	if s.serverInfo == nil {
		http.Error(w, "Server info not yet collected", http.StatusServiceUnavailable)
		return
	}

	// Create a context with timeout to prevent hanging on API calls
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Get fresh node data for display
	allNodes, err := s.nodeIPDiscovery.GetAllNodes(ctx)
	if err != nil {
		slog.Error("Failed to get current node data for homepage", "error", err)
		http.Error(w, "Failed to get current node data", http.StatusInternalServerError)
		return
	}

	// Get current node info
	currentNodeName := s.nodeIPDiscovery.GetCurrentNodeName()
	currentNodeIP, _ := s.nodeIPDiscovery.GetCurrentNodeIP(ctx)

	var currentNodeInfo *CurrentNodeInfo
	if currentNodeName != "" {
		currentNodeInfo = &CurrentNodeInfo{
			Name:   currentNodeName,
			IP:     currentNodeIP,
			Status: "healthy",
		}
	}

	// Create fresh server info with updated node data
	freshServerInfo := *s.serverInfo
	freshServerInfo.AllNodes = allNodes
	freshServerInfo.CurrentNode = currentNodeInfo

	tmpl, err := template.New("homepage").Parse(eksHomepageTemplate)
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	if err := tmpl.Execute(w, &freshServerInfo); err != nil {
		http.Error(w, "Template execution error", http.StatusInternalServerError)
		return
	}
}

func (s *EKSServer) handleInfo(w http.ResponseWriter, r *http.Request) {
	if s.serverInfo == nil {
		http.Error(w, "Server info not available", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	info := fmt.Sprintf(`{
		"aws_region": "%s",
		"cluster_name": "%s",
		"k8s_endpoint": "%s",
		"namespace": "%s",
		"node_count": %d,
		"service_count": %d
	}`,
		s.serverInfo.AWSRegion,
		s.serverInfo.ClusterName,
		s.serverInfo.K8sEndpoint,
		s.serverInfo.Namespace,
		len(s.serverInfo.AllNodes),
		len(s.serverInfo.Services))

	w.Write([]byte(info))
}

func (s *EKSServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "healthy"}`))
}
