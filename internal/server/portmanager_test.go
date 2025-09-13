package server

import (
	"net/http"
	"testing"
	"time"
)

func TestNewPortManager(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	pm := NewPortManager(handler)

	if pm == nil {
		t.Fatal("Expected PortManager, got nil")
	}
	if pm.listeners == nil {
		t.Error("Expected listeners map to be initialized")
	}
	if pm.handler == nil {
		t.Error("Expected handler to be set")
	}
}

func TestGetListeningPorts_Empty(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	pm := NewPortManager(handler)

	ports := pm.GetListeningPorts()
	if len(ports) != 0 {
		t.Errorf("Expected 0 ports, got %d", len(ports))
	}
}

func TestStartPort_DuplicatePort(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	pm := NewPortManager(handler)

	// Start first port
	err := pm.StartPort(8080)
	if err != nil {
		t.Fatalf("Expected no error starting port 8080, got %v", err)
	}

	// Give it a moment to start
	time.Sleep(10 * time.Millisecond)

	// Try to start same port again
	err = pm.StartPort(8080)
	if err == nil {
		t.Error("Expected error when starting duplicate port")
	}
	if err.Error() != "port 8080 already listening" {
		t.Errorf("Expected specific duplicate port error, got: %v", err)
	}

	// Clean up
	pm.StopAll()
}

func TestStopPort_NonExistentPort(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	pm := NewPortManager(handler)

	err := pm.StopPort(9999)
	if err == nil {
		t.Error("Expected error when stopping non-existent port")
	}
	if err.Error() != "port 9999 not listening" {
		t.Errorf("Expected specific non-existent port error, got: %v", err)
	}
}

func TestGetListeningPorts_WithPorts(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	pm := NewPortManager(handler)

	// Start some ports
	ports := []int{8081, 8082, 8083}
	for _, port := range ports {
		err := pm.StartPort(port)
		if err != nil {
			t.Fatalf("Failed to start port %d: %v", port, err)
		}
	}

	// Give them a moment to start
	time.Sleep(10 * time.Millisecond)

	listeningPorts := pm.GetListeningPorts()
	if len(listeningPorts) != len(ports) {
		t.Errorf("Expected %d listening ports, got %d", len(ports), len(listeningPorts))
	}

	// Check all ports are present (order doesn't matter)
	portMap := make(map[int]bool)
	for _, port := range listeningPorts {
		portMap[port] = true
	}

	for _, expectedPort := range ports {
		if !portMap[expectedPort] {
			t.Errorf("Expected port %d to be listening", expectedPort)
		}
	}

	// Clean up
	pm.StopAll()
}

func TestStopAll(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	pm := NewPortManager(handler)

	// Start multiple ports
	ports := []int{8084, 8085, 8086}
	for _, port := range ports {
		err := pm.StartPort(port)
		if err != nil {
			t.Fatalf("Failed to start port %d: %v", port, err)
		}
	}

	// Give them a moment to start
	time.Sleep(10 * time.Millisecond)

	// Verify ports are listening
	listeningPorts := pm.GetListeningPorts()
	if len(listeningPorts) != len(ports) {
		t.Errorf("Expected %d ports before StopAll, got %d", len(ports), len(listeningPorts))
	}

	// Stop all ports
	pm.StopAll()

	// Verify no ports are listening
	listeningPorts = pm.GetListeningPorts()
	if len(listeningPorts) != 0 {
		t.Errorf("Expected 0 ports after StopAll, got %d", len(listeningPorts))
	}
}

func TestStartStop_SinglePort(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	pm := NewPortManager(handler)

	port := 8087

	// Start port
	err := pm.StartPort(port)
	if err != nil {
		t.Fatalf("Failed to start port %d: %v", port, err)
	}

	// Give it a moment to start
	time.Sleep(10 * time.Millisecond)

	// Verify port is listening
	listeningPorts := pm.GetListeningPorts()
	if len(listeningPorts) != 1 {
		t.Errorf("Expected 1 listening port, got %d", len(listeningPorts))
	}
	if listeningPorts[0] != port {
		t.Errorf("Expected port %d, got %d", port, listeningPorts[0])
	}

	// Stop port
	err = pm.StopPort(port)
	if err != nil {
		t.Fatalf("Failed to stop port %d: %v", port, err)
	}

	// Verify port is no longer listening
	listeningPorts = pm.GetListeningPorts()
	if len(listeningPorts) != 0 {
		t.Errorf("Expected 0 listening ports after stop, got %d", len(listeningPorts))
	}
}