package server

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"k8s-node-proxy/internal/discovery"
	"k8s-node-proxy/internal/k8s"
	"k8s-node-proxy/internal/proxy"
)

type Server struct {
	portManager   *PortManager
	nodeDiscovery *k8s.NodePortDiscovery
}

func New(projectID string) (*Server, error) {
	slog.Info("Initializing k8s-node-proxy server", "project", projectID)

	nodeIPDiscovery, err := discovery.New(projectID)
	if err != nil {
		return nil, err
	}

	handler := proxy.NewHandler(nodeIPDiscovery)
	portManager := NewPortManager(handler)

	nodePortDiscovery, err := k8s.NewNodePortDiscovery(projectID)
	if err != nil {
		return nil, err
	}

	slog.Info("Server initialization completed successfully")
	return &Server{
		portManager:   portManager,
		nodeDiscovery: nodePortDiscovery,
	}, nil
}

func (s *Server) Run() error {
	ctx := context.Background()

	// Discover NodePorts once at startup
	ports, err := s.nodeDiscovery.DiscoverNodePorts(ctx)
	if err != nil {
		return err
	}

	slog.Info("Starting proxy listeners", "port_count", len(ports))

	// Start listening on all discovered ports
	for _, port := range ports {
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
