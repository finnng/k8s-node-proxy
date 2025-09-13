package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"k8s-node-proxy/internal/assets"
	"k8s-node-proxy/internal/nodes"
	"k8s-node-proxy/internal/services"
	"k8s-node-proxy/internal/proxy"
)

type Server struct {
	projectID       string
	portManager     *PortManager
	nodeDiscovery   *services.NodePortDiscovery
	nodeIPDiscovery *nodes.NodeDiscovery
	serverInfo      *ServerInfo
}

func New(projectID string) (*Server, error) {
	slog.Info("Initializing k8s-node-proxy server", "project", projectID)

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
		nodeDiscovery:   nodePortDiscovery,
		nodeIPDiscovery: nodeIPDiscovery,
		serverInfo:      nil, // Will be populated during Run()
	}

	// Create router handler
	proxyHandler := proxy.NewHandler(nodeIPDiscovery)
	routerHandler := server.createRouterHandler(proxyHandler)
	portManager := NewPortManager(routerHandler)
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

	// Always start port 80 for homepage
	if err := s.portManager.StartPort(80); err != nil {
		slog.Error("Failed to start homepage port 80", "error", err)
	}

	// Start health monitoring for node IP discovery
	s.nodeIPDiscovery.StartHealthMonitoring()
	slog.Info("Started node health monitoring")

	// Discover NodePorts once at startup
	ports, err := s.nodeDiscovery.DiscoverNodePorts(ctx)
	if err != nil {
		return err
	}

	slog.Info("Starting proxy listeners", "port_count", len(ports))

	// Start listening on all discovered ports (skip 80 if already started)
	for _, port := range ports {
		if port == 80 {
			continue // Already started above
		}
		if err := s.portManager.StartPort(port); err != nil {
			slog.Error("Failed to start port listener", "port", port, "error", err)
		}
	}

	slog.Info("All proxy listeners started successfully")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down all servers...")
	s.nodeIPDiscovery.StopHealthMonitoring()
	s.portManager.StopAll()
	slog.Info("All servers exited")
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

func (s *Server) createRouterHandler(proxyHandler *proxy.Handler) http.Handler {
	mux := http.NewServeMux()
	
	// Homepage and static assets on port 80 only
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		isPort80 := r.Host == ":80" || r.Host == "localhost:80" || r.Host == "localhost" || strings.HasSuffix(r.Host, ":80")

		if isPort80 {
			// Handle all requests on port 80 as homepage/management interface
			if r.URL.Path == "/" {
				s.handleHomepage(w, r)
				return
			}
			if r.URL.Path == "/favicon.ico" {
				w.Header().Set("Content-Type", "image/x-icon")
				w.Header().Set("Cache-Control", "public, max-age=86400") // Cache for 1 day
				w.Write(assets.FaviconICO)
				return
			}
			// Block all other requests on port 80 - DO NOT proxy them!
			http.Error(w, "Not Found - This is the management interface on port 80", http.StatusNotFound)
			return
		}

		// Only proxy requests on NodePort ports (not port 80)
		proxyHandler.ServeHTTP(w, r)
	})
	
	return mux
}
