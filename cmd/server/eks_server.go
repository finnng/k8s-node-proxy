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
	"k8s-node-proxy/internal/server"
	"k8s-node-proxy/internal/services"
)

// EKSServerInfo contains information about the EKS server and cluster
type EKSServerInfo struct {
	AWSRegion   string
	ClusterName string
	K8sEndpoint string
	Namespace   string
	NodeIPs     []string
	Services    []services.ServiceInfo
	CurrentNode *server.CurrentNodeInfo
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

	// Trigger initial node selection (with timeout to prevent hanging)
	nodeCtx, nodeCancel := context.WithTimeout(ctx, 10*time.Second)
	if _, err := s.nodeIPDiscovery.GetCurrentNodeIP(nodeCtx); err != nil {
		slog.Warn("Failed to select initial node, will retry via health monitoring", "error", err)
	} else {
		slog.Info("Initial node selected", "node", s.nodeIPDiscovery.GetCurrentNodeName())
	}
	nodeCancel()

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
	slog.Info("Shutting down EKS server...")

	// Stop health monitoring
	slog.Info("Stopping health monitoring...")
	s.nodeIPDiscovery.StopHealthMonitoring()

	// Stop all ports
	slog.Info("Health monitoring stopped, stopping port listeners...")
	s.portManager.StopAll()

	slog.Info("EKS server shutdown complete")
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

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			s.handleHomepage(w, r)
			return
		}
		if path == "/health" {
			s.handleHealth(w, r)
			return
		}

		// Block all other requests on service port - DO NOT proxy them!
		http.Error(w, fmt.Sprintf("Not Found - This is the management interface on port %d", s.servicePort), http.StatusNotFound)
	})

	return mux
}

func (s *EKSServer) handleHomepage(w http.ResponseWriter, r *http.Request) {
	if s.serverInfo == nil {
		http.Error(w, "Server info not yet collected", http.StatusServiceUnavailable)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	allNodes, err := s.nodeIPDiscovery.GetAllNodes(ctx)
	if err != nil {
		slog.Error("Failed to get current node data for homepage", "error", err)
		http.Error(w, "Failed to get current node data", http.StatusInternalServerError)
		return
	}

	currentNodeName := s.nodeIPDiscovery.GetCurrentNodeName()
	currentNodeIP, _ := s.nodeIPDiscovery.GetCurrentNodeIP(ctx)

	var currentNodeInfo *server.CurrentNodeInfo
	if currentNodeName != "" {
		currentNodeInfo = &server.CurrentNodeInfo{
			Name:   currentNodeName,
			IP:     currentNodeIP,
			Status: "healthy",
		}
	}

	clusterInfo := []server.ClusterInfoField{
		{Key: "AWS Region", Value: s.serverInfo.AWSRegion},
		{Key: "Cluster Name", Value: s.serverInfo.ClusterName},
		{Key: "Kubernetes Endpoint", Value: s.serverInfo.K8sEndpoint},
		{Key: "Target Namespace", Value: s.serverInfo.Namespace},
	}

	data := server.HomepageData{
		PlatformName: "Amazon EKS",
		ClusterInfo:  clusterInfo,
		Namespace:    s.serverInfo.Namespace,
		CurrentNode:  currentNodeInfo,
		AllNodes:     allNodes,
		Services:     s.serverInfo.Services,
	}

	tmpl, err := template.New("homepage").Parse(server.HomepageTemplate)
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	if err := tmpl.Execute(w, &data); err != nil {
		http.Error(w, "Template execution error", http.StatusInternalServerError)
		return
	}
}

func (s *EKSServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	currentNodeName := s.nodeIPDiscovery.GetCurrentNodeName()

	response := fmt.Sprintf(`{
		"proxy_server": "healthy",
		"current_node_name": "%s"
	}`, currentNodeName)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(response))
}
