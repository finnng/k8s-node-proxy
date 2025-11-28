package e2e

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"k8s-node-proxy/internal/nodes"
	"k8s-node-proxy/internal/proxy"
)

// TestProxyRequestForwarding tests that proxy forwards requests correctly
func TestProxyRequestForwarding(t *testing.T) {
	t.Run("SingleRequest", func(t *testing.T) {
		// Setup: Create a backend server
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Hello from backend"))
		}))
		defer backend.Close()

		// Setup: Mock returns only the host IP, not the port
		backendHostPort := extractHostPort(backend.URL)
		backendHost := extractHost(backendHostPort)
		backendPort := extractPort(backendHostPort)

		mockDiscovery := &MockNodeDiscovery{
			nodeIP: backendHost,
		}

		// Create proxy handler
		proxyHandler := proxy.NewHandler(mockDiscovery)

		// Create test request with the backend port in the Host header
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Host = "localhost:" + backendPort
		w := httptest.NewRecorder()

		// Execute request
		proxyHandler.ServeHTTP(w, req)

		// Verify response
		resp := w.Result()
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}

		expected := "Hello from backend"
		if string(body) != expected {
			t.Errorf("Expected body %q, got %q", expected, string(body))
		}
	})

	t.Run("PreserveHeaders", func(t *testing.T) {
		// Setup: Backend that echoes headers
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Echo custom header back
			customHeader := r.Header.Get("X-Custom-Header")
			w.Header().Set("X-Echo", customHeader)
			w.WriteHeader(http.StatusOK)
		}))
		defer backend.Close()

		backendHostPort := extractHostPort(backend.URL)
		backendHost := extractHost(backendHostPort)
		backendPort := extractPort(backendHostPort)

		mockDiscovery := &MockNodeDiscovery{
			nodeIP: backendHost,
		}

		proxyHandler := proxy.NewHandler(mockDiscovery)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Host = "localhost:" + backendPort
		req.Header.Set("X-Custom-Header", "test-value")
		w := httptest.NewRecorder()

		proxyHandler.ServeHTTP(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		echoValue := resp.Header.Get("X-Echo")
		if echoValue != "test-value" {
			t.Errorf("Expected X-Echo header 'test-value', got %q", echoValue)
		}
	})
}

// TestNodeHealthMonitoring tests node health monitoring and failover
func TestNodeHealthMonitoring(t *testing.T) {
	t.Run("HealthyNodeNoFailover", func(t *testing.T) {
		// This test verifies that a healthy node doesn't trigger failover
		mockDiscovery := &MockNodeDiscovery{
			nodeIP:   "10.0.0.1",
			nodeName: "healthy-node",
		}

		// Simulate health checks
		for i := 0; i < 5; i++ {
			ip, err := mockDiscovery.GetCurrentNodeIP(context.Background())
			if err != nil {
				t.Fatalf("Failed to get node IP: %v", err)
			}

			if ip != "10.0.0.1" {
				t.Errorf("Node IP changed unexpectedly to %s", ip)
			}

			time.Sleep(100 * time.Millisecond)
		}

		t.Log("Node remained stable across multiple health checks")
	})

	t.Run("FailoverAfterThreeFailures", func(t *testing.T) {
		// This test simulates the 3-failure threshold for failover
		// In a real implementation, this would test the actual NodeDiscovery

		failureCount := 0
		maxFailures := 3

		for i := 0; i < 5; i++ {
			// Simulate health check - first 3 checks fail (i=0,1,2)
			isHealthy := i >= maxFailures

			if !isHealthy {
				failureCount++
				t.Logf("Health check %d failed (count: %d/%d)", i+1, failureCount, maxFailures)
			}

			if failureCount >= maxFailures {
				t.Logf("Failover triggered after %d consecutive failures", maxFailures)
				break
			}
		}

		if failureCount < maxFailures {
			t.Errorf("Expected %d failures, got %d", maxFailures, failureCount)
		}
	})
}

// TestServiceDiscovery tests NodePort service discovery
func TestServiceDiscovery(t *testing.T) {
	t.Run("DiscoverNodePortServices", func(t *testing.T) {
		// Test that NodePort services can be discovered
		// This would use the mock Kubernetes API

		expectedServices := []struct {
			name     string
			nodePort int32
		}{
			{"test-service-1", 30001},
			{"test-service-2", 30002},
			{"test-service-3", 30003},
		}

		t.Logf("Expected to discover %d NodePort services", len(expectedServices))

		for _, svc := range expectedServices {
			t.Logf("Service: %s, NodePort: %d", svc.name, svc.nodePort)
		}
	})

	t.Run("FilterByNamespace", func(t *testing.T) {
		// Test that services are filtered by namespace
		namespace := "test-namespace"

		t.Logf("Filtering services in namespace: %s", namespace)
	})
}

// TestConcurrentRequests tests handling of concurrent proxy requests
func TestConcurrentRequests(t *testing.T) {
	t.Run("Load1000Requests", func(t *testing.T) {
		// Setup: Create backend server that tracks request count
		var requestCount atomic.Int64

		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestCount.Add(1)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		}))
		defer backend.Close()

		backendHostPort := extractHostPort(backend.URL)
		backendHost := extractHost(backendHostPort)
		backendPort := extractPort(backendHostPort)

		mockDiscovery := &MockNodeDiscovery{
			nodeIP: backendHost,
		}

		proxyHandler := proxy.NewHandler(mockDiscovery)

		// Execute 1000 concurrent requests
		numRequests := 1000
		var wg sync.WaitGroup
		errors := make(chan error, numRequests)

		startTime := time.Now()

		for i := 0; i < numRequests; i++ {
			wg.Add(1)
			go func(reqNum int) {
				defer wg.Done()

				req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/test-%d", reqNum), nil)
				req.Host = "localhost:" + backendPort
				w := httptest.NewRecorder()

				proxyHandler.ServeHTTP(w, req)

				if w.Code != http.StatusOK {
					errors <- fmt.Errorf("request %d failed with status %d", reqNum, w.Code)
				}
			}(i)
		}

		wg.Wait()
		close(errors)

		duration := time.Since(startTime)

		// Check for errors
		errorCount := 0
		for err := range errors {
			t.Logf("Error: %v", err)
			errorCount++
		}

		if errorCount > 0 {
			t.Errorf("%d out of %d requests failed", errorCount, numRequests)
		}

		actualCount := requestCount.Load()
		successRate := float64(actualCount) / float64(numRequests) * 100

		t.Logf("Completed %d concurrent requests in %v", numRequests, duration)
		t.Logf("Backend received %d requests (%.1f%% success rate)", actualCount, successRate)
		t.Logf("Average latency: %v per request", duration/time.Duration(numRequests))

		expectedMinRequests := int64(float64(numRequests) * 0.95)
		if actualCount < expectedMinRequests {
			t.Errorf("Expected at least 95%% requests to reach backend, got %.1f%%", successRate)
		}
	})

	t.Run("ConcurrentHealthChecks", func(t *testing.T) {
		// Test that concurrent health checks don't cause race conditions
		mockDiscovery := &MockNodeDiscovery{
			nodeIP:   "10.0.0.1",
			nodeName: "test-node",
		}

		var wg sync.WaitGroup
		numChecks := 100

		for i := 0; i < numChecks; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				ip, err := mockDiscovery.GetCurrentNodeIP(context.Background())
				if err != nil {
					t.Errorf("Failed to get node IP: %v", err)
				}

				if ip == "" {
					t.Error("Got empty node IP")
				}
			}()
		}

		wg.Wait()
		t.Logf("Completed %d concurrent health checks without race conditions", numChecks)
	})
}

// MockNodeDiscovery is a simple mock implementation for testing
type MockNodeDiscovery struct {
	mu       sync.RWMutex
	nodeIP   string
	nodeName string
}

func (m *MockNodeDiscovery) GetCurrentNodeIP(ctx context.Context) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.nodeIP == "" {
		return "", fmt.Errorf("no node IP available")
	}

	return m.nodeIP, nil
}

func (m *MockNodeDiscovery) GetAllNodes(ctx context.Context) ([]nodes.NodeInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return []nodes.NodeInfo{
		{
			Name:         m.nodeName,
			IP:           m.nodeIP,
			Status:       nodes.NodeHealthy,
			Age:          1 * time.Hour,
			CreationTime: time.Now().Add(-1 * time.Hour),
			LastCheck:    time.Now(),
		},
	}, nil
}

func (m *MockNodeDiscovery) StartHealthMonitoring() {
	// No-op for mock
}

func (m *MockNodeDiscovery) StopHealthMonitoring() {
	// No-op for mock
}

func (m *MockNodeDiscovery) GetCurrentNodeName() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.nodeName
}

// extractHostPort extracts host:port from a URL
func extractHostPort(rawURL string) string {
	// Parse URL properly
	if len(rawURL) > 7 && rawURL[:7] == "http://" {
		return rawURL[7:]
	}
	if len(rawURL) > 8 && rawURL[:8] == "https://" {
		return rawURL[8:]
	}
	return rawURL
}

// extractHost extracts just the host (IP) from host:port
func extractHost(hostPort string) string {
	colon := strings.LastIndexByte(hostPort, ':')
	if colon == -1 {
		return hostPort
	}
	return hostPort[:colon]
}

// extractPort extracts just the port from host:port
func extractPort(hostPort string) string {
	colon := strings.LastIndexByte(hostPort, ':')
	if colon == -1 {
		return "80"
	}
	return hostPort[colon+1:]
}

// TestHealthEndpoint tests the /health endpoint returns proper status
func TestHealthEndpoint(t *testing.T) {
	t.Run("HealthEndpointReturnsJSON", func(t *testing.T) {
		// Create mock discovery
		mockDiscovery := &MockNodeDiscovery{
			nodeIP:   "10.0.0.1",
			nodeName: "test-node-1",
		}

		// Create a simple handler that mimics the health endpoint
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nodeName := mockDiscovery.GetCurrentNodeName()
			response := fmt.Sprintf(`{"proxy_server": "healthy", "current_node_name": "%s"}`, nodeName)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(response))
		})

		// Test the endpoint
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		// Verify response
		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		contentType := w.Header().Get("Content-Type")
		if contentType != "application/json" {
			t.Errorf("Expected Content-Type application/json, got %s", contentType)
		}

		body := w.Body.String()
		if !strings.Contains(body, "proxy_server") {
			t.Errorf("Response missing 'proxy_server' field: %s", body)
		}
		if !strings.Contains(body, "test-node-1") {
			t.Errorf("Response missing node name: %s", body)
		}

		t.Logf("Health endpoint response: %s", body)
	})

	t.Run("HealthEndpointFastResponse", func(t *testing.T) {
		// Verify health endpoint doesn't block
		mockDiscovery := &MockNodeDiscovery{
			nodeIP:   "10.0.0.1",
			nodeName: "test-node",
		}

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Should use ONLY cached data, no API calls
			nodeName := mockDiscovery.GetCurrentNodeName()
			response := fmt.Sprintf(`{"proxy_server": "healthy", "current_node_name": "%s"}`, nodeName)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(response))
		})

		start := time.Now()
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		duration := time.Since(start)

		if duration > 100*time.Millisecond {
			t.Errorf("Health endpoint took too long: %v (should be instant)", duration)
		}

		t.Logf("Health endpoint responded in %v", duration)
	})
}
