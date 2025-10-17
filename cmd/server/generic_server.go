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
	"k8s-node-proxy/internal/services"
)

const homepageTemplate = `
<!DOCTYPE html>
<html>
<head>
    <title>k8s-node-proxy</title>
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
    <h1>k8s-node-proxy Server (Generic Kubernetes)</h1>
    
    <div class="section">
        <h2>Cluster Information</h2>
        <table>
            <tr><th>Property</th><th>Value</th></tr>
            <tr><td>Project ID</td><td>{{.ProjectID}}</td></tr>
            <tr><td>Cluster Name</td><td>{{.ClusterName}}</td></tr>
            <tr><td>Cluster Location</td><td>{{.ClusterLocation}}</td></tr>
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
	CurrentNode     *CurrentNodeInfo
	AllNodes        []nodes.NodeInfo
}

type CurrentNodeInfo struct {
	Name   string
	IP     string
	Status string
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
	slog.Info("Shutting down server...")

	// Stop health monitoring
	s.nodeIPDiscovery.StopHealthMonitoring()

	// Stop all ports
	s.portManager.StopAll()

	slog.Info("Server shutdown complete")
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

	// Homepage
	mux.HandleFunc("/", s.handleHomepage)

	// Info endpoint
	mux.HandleFunc("/info", s.handleInfo)

	return mux
}

func (s *GenericServer) handleHomepage(w http.ResponseWriter, r *http.Request) {
	if s.serverInfo == nil {
		http.Error(w, "Server info not yet collected", http.StatusServiceUnavailable)
		return
	}

	// Create a context with timeout to prevent hanging on API calls
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Get fresh node data for display (use cached data from serverInfo if this fails)
	allNodes, err := s.nodeIPDiscovery.GetAllNodes(ctx)
	if err != nil {
		slog.Warn("Failed to get fresh node data for homepage, using cached data", "error", err)
		allNodes = s.serverInfo.AllNodes
	}

	// Get current node info - don't call GetCurrentNodeIP as it might trigger discovery
	currentNodeName := s.nodeIPDiscovery.GetCurrentNodeName()
	var currentNodeInfo *CurrentNodeInfo
	if currentNodeName != "" {
		// Try to get IP with short timeout, but don't fail if it's not available yet
		currentNodeIP, err := s.nodeIPDiscovery.GetCurrentNodeIP(ctx)
		if err == nil {
			currentNodeInfo = &CurrentNodeInfo{
				Name:   currentNodeName,
				IP:     currentNodeIP,
				Status: "healthy", // Will be updated by health monitoring
			}
		}
	}

	// Create fresh server info with updated node data
	freshServerInfo := *s.serverInfo
	freshServerInfo.AllNodes = allNodes
	freshServerInfo.CurrentNode = currentNodeInfo

	tmpl, err := template.New("homepage").Parse(homepageTemplate)
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

func (s *GenericServer) handleInfo(w http.ResponseWriter, r *http.Request) {
	if s.serverInfo == nil {
		http.Error(w, "Server info not available", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	info := fmt.Sprintf(`{
		"project_id": "%s",
		"cluster_name": "%s",
		"cluster_location": "%s",
		"k8s_endpoint": "%s",
		"namespace": "%s",
		"node_count": %d,
		"service_count": %d
	}`,
		s.serverInfo.ProjectID,
		s.serverInfo.ClusterName,
		s.serverInfo.ClusterLocation,
		s.serverInfo.K8sEndpoint,
		s.serverInfo.Namespace,
		len(s.serverInfo.AllNodes),
		len(s.serverInfo.Services))

	w.Write([]byte(info))
}
