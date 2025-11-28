package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s-node-proxy/internal/assets"
	"k8s-node-proxy/internal/nodes"
	"k8s-node-proxy/internal/proxy"
	"k8s-node-proxy/internal/services"
)

type Server struct {
	projectID       string
	servicePort     int
	portManager     *PortManager
	nodeDiscovery   *services.NodePortDiscovery
	nodeIPDiscovery *nodes.NodeDiscovery
	serverInfo      *ServerInfo
}

func New(projectID string, servicePort int) (*Server, error) {
	slog.Info("Initializing k8s-node-proxy server", "project", projectID, "service_port", servicePort)

	nodeIPDiscovery, err := nodes.New(projectID)
	if err != nil {
		return nil, err
	}

	nodePortDiscovery, err := services.NewNodePortDiscovery(projectID)
	if err != nil {
		return nil, err
	}

	server := &Server{
		projectID:       projectID,
		servicePort:     servicePort,
		nodeDiscovery:   nodePortDiscovery,
		nodeIPDiscovery: nodeIPDiscovery,
		serverInfo:      nil, // Will be populated during Run()
	}

	// Create port manager
	portManager := NewPortManager()
	server.portManager = portManager

	slog.Info("Server initialization completed successfully")
	return server, nil
}

func (s *Server) Run() error {
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

	slog.Info("Starting proxy listeners", "port_count", len(ports))

	// Start listening on all discovered ports (skip service port if already started)
	for _, port := range ports {
		if port == s.servicePort {
			continue // Already started above
		}
		if err := s.portManager.StartPort(port, proxyHandler); err != nil {
			slog.Error("Failed to start port listener", "port", port, "error", err)
		}
	}

	slog.Info("All proxy listeners started successfully")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down server...")
	slog.Info("Stopping health monitoring...")
	s.nodeIPDiscovery.StopHealthMonitoring()
	slog.Info("Health monitoring stopped, stopping port listeners...")
	s.portManager.StopAll()
	slog.Info("Server shutdown complete")
	return nil
}

func (s *Server) collectServerInfo(ctx context.Context) error {
	slog.Info("Collecting server information")

	// Get cluster info
	clusterInfo := s.nodeDiscovery.GetClusterInfo()

	// Get services info
	services, err := s.nodeDiscovery.DiscoverServices(ctx)
	if err != nil {
		return fmt.Errorf("failed to discover services: %w", err)
	}

	// Get node IPs
	nodeIPs, err := s.getAllNodeIPs(ctx)
	if err != nil {
		return fmt.Errorf("failed to get node IPs: %w", err)
	}

	// Get detailed node information
	allNodes, err := s.nodeIPDiscovery.GetAllNodes(ctx)
	if err != nil {
		return fmt.Errorf("failed to get all nodes: %w", err)
	}

	// Get current node info
	currentNodeName := s.nodeIPDiscovery.GetCurrentNodeName()
	currentNodeIP, _ := s.nodeIPDiscovery.GetCurrentNodeIP(ctx)

	var currentNodeInfo *CurrentNodeInfo
	if currentNodeName != "" {
		currentNodeInfo = &CurrentNodeInfo{
			Name:   currentNodeName,
			IP:     currentNodeIP,
			Status: "healthy", // Will be updated by health monitoring
		}
	}

	s.serverInfo = &ServerInfo{
		ProjectID:       s.projectID,
		ClusterName:     clusterInfo.Name,
		ClusterLocation: clusterInfo.Location,
		K8sEndpoint:     clusterInfo.Endpoint,
		Namespace:       os.Getenv("NAMESPACE"),
		NodeIPs:         nodeIPs,
		Services:        services,
		CurrentNode:     currentNodeInfo,
		AllNodes:        allNodes,
	}

	slog.Info("Server information collected successfully")
	return nil
}

func (s *Server) getAllNodeIPs(ctx context.Context) ([]string, error) {
	// For now, just get the current node IP
	// Could be enhanced to get all node IPs
	nodeIP, err := s.nodeIPDiscovery.GetCurrentNodeIP(ctx)
	if err != nil {
		return nil, err
	}
	return []string{nodeIP}, nil
}

func (s *Server) createServiceHandler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			s.handleHomepage(w, r)
			return
		}
		if path == "/favicon.ico" {
			w.Header().Set("Content-Type", "image/x-icon")
			w.Header().Set("Cache-Control", "public, max-age=86400") // Cache for 1 day
			w.Write(assets.FaviconICO)
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

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Use ONLY cached data - NO API calls, NO blocking
	currentNodeName := s.nodeIPDiscovery.GetCurrentNodeName()

	// Build simple response with cached info only
	response := fmt.Sprintf(`{
		"proxy_server": "healthy",
		"current_node_name": "%s"
	}`, currentNodeName)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(response))
}
