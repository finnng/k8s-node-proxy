package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"k8s-node-proxy/internal/discovery"
	"k8s-node-proxy/internal/k8s"
	"k8s-node-proxy/internal/proxy"
)

type Server struct {
	projectID       string
	portManager     *PortManager
	nodeDiscovery   *k8s.NodePortDiscovery
	nodeIPDiscovery *discovery.NodeDiscovery
	serverInfo      *ServerInfo
}

func New(projectID string) (*Server, error) {
	slog.Info("Initializing k8s-node-proxy server", "project", projectID)

	nodeIPDiscovery, err := discovery.New(projectID)
	if err != nil {
		return nil, err
	}

	nodePortDiscovery, err := k8s.NewNodePortDiscovery(projectID)
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

	// Always start port 8080 for homepage
	if err := s.portManager.StartPort(8080); err != nil {
		slog.Error("Failed to start homepage port 8080", "error", err)
	}

	// Discover NodePorts once at startup
	ports, err := s.nodeDiscovery.DiscoverNodePorts(ctx)
	if err != nil {
		return err
	}

	slog.Info("Starting proxy listeners", "port_count", len(ports))

	// Start listening on all discovered ports (skip 8080 if already started)
	for _, port := range ports {
		if port == 8080 {
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

	s.serverInfo = &ServerInfo{
		ProjectID:       s.projectID,
		ClusterName:     clusterInfo.Name,
		ClusterLocation: clusterInfo.Location,
		K8sEndpoint:     clusterInfo.Endpoint,
		NodeIPs:         nodeIPs,
		Services:        services,
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
	
	// Homepage on root path for port 8080 only
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" && (r.Host == ":8080" || r.Host == "localhost:8080") {
			s.handleHomepage(w, r)
			return
		}
		// All other requests go to proxy
		proxyHandler.ServeHTTP(w, r)
	})
	
	return mux
}
