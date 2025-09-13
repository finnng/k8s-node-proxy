package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"k8s-node-proxy/internal/proxy"
)

type PortListener struct {
	port     int
	server   *http.Server
	shutdown chan struct{}
	done     chan struct{}
}

type PortManager struct {
	listeners map[int]*PortListener
	mutex     sync.RWMutex
	handler   *proxy.Handler
}

func NewPortManager(handler *proxy.Handler) *PortManager {
	return &PortManager{
		listeners: make(map[int]*PortListener),
		handler:   handler,
	}
}

func (pm *PortManager) StartPort(port int) error {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	if _, exists := pm.listeners[port]; exists {
		return fmt.Errorf("port %d already listening", port)
	}

	listener := &PortListener{
		port:     port,
		server:   &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: pm.handler},
		shutdown: make(chan struct{}),
		done:     make(chan struct{}),
	}

	go listener.start()
	pm.listeners[port] = listener
	slog.Info("Started listening on port", "port", port)
	return nil
}

func (pm *PortManager) StopPort(port int) error {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	listener, exists := pm.listeners[port]
	if !exists {
		return fmt.Errorf("port %d not listening", port)
	}

	close(listener.shutdown)
	<-listener.done
	delete(pm.listeners, port)
	slog.Info("Stopped listening on port", "port", port)
	return nil
}

func (pm *PortManager) GetListeningPorts() []int {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	var ports []int
	for port := range pm.listeners {
		ports = append(ports, port)
	}
	return ports
}

func (pm *PortManager) StopAll() {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

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
