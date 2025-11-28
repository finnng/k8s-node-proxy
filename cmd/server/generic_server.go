package main

import (
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"k8s-node-proxy/internal/nodes"
	"k8s-node-proxy/internal/proxy"
	"k8s-node-proxy/internal/server"
	"k8s-node-proxy/internal/services"
)

// PortListener manages a single port listener
type PortListener struct {
	port     int
	server   *http.Server
	shutdown chan struct{}
	done     chan struct{}
}

// PortManager manages multiple port listeners
type PortManager struct {
	listeners map[int]*PortListener
}

// NewPortManager creates a new port manager
func NewPortManager() *PortManager {
	return &PortManager{
		listeners: make(map[int]*PortListener),
	}
}

// StartPort starts listening on the specified port with the given handler
func (pm *PortManager) StartPort(port int, handler http.Handler) error {
	if _, exists := pm.listeners[port]; exists {
		return fmt.Errorf("port %d already listening", port)
	}

	listener := &PortListener{
		port:     port,
		server:   &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: handler},
		shutdown: make(chan struct{}),
		done:     make(chan struct{}),
	}

	go listener.start()
	pm.listeners[port] = listener
	slog.Info("Started listening on port", "port", port)
	return nil
}

// StopAll stops all port listeners
func (pm *PortManager) StopAll() {
	var wg sync.WaitGroup
	for port, listener := range pm.listeners {
		wg.Add(1)
		go func(p int, l *PortListener) {
			defer wg.Done()
			close(l.shutdown)
			<-l.done
			slog.Info("Stopped listening on port", "port", p)
		}(port, listener)
	}
	wg.Wait()
	pm.listeners = make(map[int]*PortListener)
}

// start starts the port listener
func (l *PortListener) start() {
	defer close(l.done)

	go func() {
		if err := l.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Port server error", "port", l.port, "error", err)
		}
	}()

	<-l.shutdown

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := l.server.Shutdown(ctx); err != nil {
		slog.Error("Port forced shutdown", "port", l.port, "error", err)
	}
}

// ServerInfo contains information about the server and cluster
type ServerInfo struct {
	ProjectID       string
	ClusterName     string
	ClusterLocation string
	K8sEndpoint     string
	Namespace       string
	NodeIPs         []string
	Services        []services.ServiceInfo
	CurrentNode     *server.CurrentNodeInfo
	AllNodes        []nodes.NodeInfo
}

// GenericServer is a server implementation for generic Kubernetes clusters
type GenericServer struct {
	servicePort     int
	portManager     *PortManager
	nodeDiscovery   *services.GenericNodePortDiscovery
	nodeIPDiscovery *nodes.GenericNodeDiscovery
	serverInfo      *ServerInfo
}

// NewGenericServer creates a new generic server
func NewGenericServer(servicePort int) (*GenericServer, error) {
	slog.Info("Initializing k8s-node-proxy server for generic Kubernetes", "service_port", servicePort)

	nodePortDiscovery, err := services.NewGenericNodePortDiscovery()
	if err != nil {
		return nil, fmt.Errorf("failed to create generic service discovery: %w", err)
	}

	// Get the clientset from the service discovery to pass to node discovery
	// We need to access the private field, so we'll create the node discovery with the same clientset
	nodeIPDiscovery, err := nodes.NewGenericNodeDiscovery(nodePortDiscovery.GetClientset())
	if err != nil {
		return nil, fmt.Errorf("failed to create generic node discovery: %w", err)
	}

	server := &GenericServer{
		servicePort:     servicePort,
		nodeDiscovery:   nodePortDiscovery,
		nodeIPDiscovery: nodeIPDiscovery,
		serverInfo:      nil, // Will be populated during Run()
	}

	// Create port manager
	portManager := NewPortManager()
	server.portManager = portManager

	slog.Info("Generic server initialization completed successfully")
	return server, nil
}

func (s *GenericServer) Run() error {
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

	slog.Info("k8s-node-proxy server started successfully", "service_port", s.servicePort)

	<-c
	slog.Info("Shutting down Generic server...")

	// Stop health monitoring
	slog.Info("Stopping health monitoring...")
	s.nodeIPDiscovery.StopHealthMonitoring()

	// Stop all ports
	slog.Info("Health monitoring stopped, stopping port listeners...")
	s.portManager.StopAll()

	slog.Info("Generic server shutdown complete")
	return nil
}

func (s *GenericServer) collectServerInfo(ctx context.Context) error {
	slog.Info("Collecting server information")

	clusterInfo := s.nodeDiscovery.GetClusterInfo()

	// Get services info
	services, err := s.nodeDiscovery.DiscoverServices(ctx)
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

	s.serverInfo = &ServerInfo{
		ProjectID:       "generic",
		ClusterName:     clusterInfo.Name,
		ClusterLocation: clusterInfo.Location,
		K8sEndpoint:     clusterInfo.Endpoint,
		Namespace:       os.Getenv("NAMESPACE"),
		NodeIPs:         nodeIPs,
		Services:        services,
		AllNodes:        allNodes,
	}

	return nil
}

func (s *GenericServer) createServiceHandler() http.Handler {
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

func (s *GenericServer) handleHomepage(w http.ResponseWriter, r *http.Request) {
	if s.serverInfo == nil {
		http.Error(w, "Server info not yet collected", http.StatusServiceUnavailable)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	allNodes, err := s.nodeIPDiscovery.GetAllNodes(ctx)
	if err != nil {
		slog.Warn("Failed to get fresh node data for homepage, using cached data", "error", err)
		allNodes = s.serverInfo.AllNodes
	}

	currentNodeName := s.nodeIPDiscovery.GetCurrentNodeName()
	var currentNodeInfo *server.CurrentNodeInfo
	if currentNodeName != "" {
		currentNodeIP, err := s.nodeIPDiscovery.GetCurrentNodeIP(ctx)
		if err == nil {
			currentNodeInfo = &server.CurrentNodeInfo{
				Name:   currentNodeName,
				IP:     currentNodeIP,
				Status: "healthy",
			}
		}
	}

	clusterInfo := []server.ClusterInfoField{
		{Key: "Project ID", Value: s.serverInfo.ProjectID},
		{Key: "Cluster Name", Value: s.serverInfo.ClusterName},
		{Key: "Cluster Location", Value: s.serverInfo.ClusterLocation},
		{Key: "Kubernetes Endpoint", Value: s.serverInfo.K8sEndpoint},
		{Key: "Target Namespace", Value: s.serverInfo.Namespace},
	}

	data := server.HomepageData{
		PlatformName: "Generic Kubernetes",
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

func (s *GenericServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	currentNodeName := s.nodeIPDiscovery.GetCurrentNodeName()

	response := fmt.Sprintf(`{
		"proxy_server": "healthy",
		"current_node_name": "%s"
	}`, currentNodeName)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(response))
}
