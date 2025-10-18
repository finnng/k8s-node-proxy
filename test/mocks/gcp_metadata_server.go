package mocks

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
)

// GCPMetadataServer mocks the GCP metadata service
// Used for testing platform detection and GCE instance metadata retrieval
type GCPMetadataServer struct {
	server      *httptest.Server
	mu          sync.RWMutex
	projectID   string
	zone        string
	instanceID  string
	attributes  map[string]string
	shouldFail  bool
	failureCode int
}

// NewGCPMetadataServer creates a new mock GCP metadata server
func NewGCPMetadataServer() *GCPMetadataServer {
	mock := &GCPMetadataServer{
		projectID:  "test-project-12345",
		zone:       "us-central1-a",
		instanceID: "1234567890123456789",
		attributes: make(map[string]string),
	}

	mux := http.NewServeMux()

	// Standard metadata endpoints
	mux.HandleFunc("/computeMetadata/v1/project/project-id", mock.handleProjectID)
	mux.HandleFunc("/computeMetadata/v1/instance/zone", mock.handleZone)
	mux.HandleFunc("/computeMetadata/v1/instance/id", mock.handleInstanceID)
	mux.HandleFunc("/computeMetadata/v1/instance/attributes/", mock.handleAttributes)

	// Root endpoint for detection
	mux.HandleFunc("/", mock.handleRoot)

	mock.server = httptest.NewServer(mux)
	return mock
}

// URL returns the base URL of the mock server
func (m *GCPMetadataServer) URL() string {
	return m.server.URL
}

// Close shuts down the mock server
func (m *GCPMetadataServer) Close() {
	m.server.Close()
}

// SetProjectID sets the project ID to be returned
func (m *GCPMetadataServer) SetProjectID(projectID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.projectID = projectID
}

// SetZone sets the zone to be returned
func (m *GCPMetadataServer) SetZone(zone string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.zone = zone
}

// SetInstanceID sets the instance ID to be returned
func (m *GCPMetadataServer) SetInstanceID(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.instanceID = id
}

// SetAttribute sets a custom instance attribute
func (m *GCPMetadataServer) SetAttribute(key, value string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.attributes[key] = value
}

// SetShouldFail configures the server to return errors
func (m *GCPMetadataServer) SetShouldFail(fail bool, code int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shouldFail = fail
	m.failureCode = code
}

// handleRoot handles the root endpoint for metadata server detection
func (m *GCPMetadataServer) handleRoot(w http.ResponseWriter, r *http.Request) {
	if !m.validateMetadataHeader(r, w) {
		return
	}

	w.Header().Set("Metadata-Flavor", "Google")
	w.WriteHeader(http.StatusOK)
}

// handleProjectID handles the project ID metadata endpoint
func (m *GCPMetadataServer) handleProjectID(w http.ResponseWriter, r *http.Request) {
	if !m.validateMetadataHeader(r, w) {
		return
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.shouldFail {
		http.Error(w, "metadata server error", m.failureCode)
		return
	}

	w.Header().Set("Metadata-Flavor", "Google")
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(m.projectID))
}

// handleZone handles the zone metadata endpoint
func (m *GCPMetadataServer) handleZone(w http.ResponseWriter, r *http.Request) {
	if !m.validateMetadataHeader(r, w) {
		return
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.shouldFail {
		http.Error(w, "metadata server error", m.failureCode)
		return
	}

	w.Header().Set("Metadata-Flavor", "Google")
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	// Return zone in format: projects/PROJECT_NUM/zones/ZONE
	w.Write([]byte(fmt.Sprintf("projects/123456789/zones/%s", m.zone)))
}

// handleInstanceID handles the instance ID metadata endpoint
func (m *GCPMetadataServer) handleInstanceID(w http.ResponseWriter, r *http.Request) {
	if !m.validateMetadataHeader(r, w) {
		return
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.shouldFail {
		http.Error(w, "metadata server error", m.failureCode)
		return
	}

	w.Header().Set("Metadata-Flavor", "Google")
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(m.instanceID))
}

// handleAttributes handles custom instance attributes
func (m *GCPMetadataServer) handleAttributes(w http.ResponseWriter, r *http.Request) {
	if !m.validateMetadataHeader(r, w) {
		return
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.shouldFail {
		http.Error(w, "metadata server error", m.failureCode)
		return
	}

	// Extract attribute key from path
	// Path format: /computeMetadata/v1/instance/attributes/KEY
	key := r.URL.Path[len("/computeMetadata/v1/instance/attributes/"):]

	value, exists := m.attributes[key]
	if !exists {
		http.Error(w, "attribute not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Metadata-Flavor", "Google")
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(value))
}

// validateMetadataHeader validates the required Metadata-Flavor header
// GCP metadata service requires Metadata-Flavor: Google header
func (m *GCPMetadataServer) validateMetadataHeader(r *http.Request, w http.ResponseWriter) bool {
	flavor := r.Header.Get("Metadata-Flavor")
	if flavor != "Google" {
		http.Error(w, "Missing or invalid Metadata-Flavor header", http.StatusForbidden)
		return false
	}
	return true
}

// GetRequestCount returns the number of requests received (useful for testing)
type RequestCounter struct {
	mu    sync.RWMutex
	count int
}

// GCPMetadataResponse represents a structured metadata response
type GCPMetadataResponse struct {
	ProjectID  string            `json:"project_id"`
	Zone       string            `json:"zone"`
	InstanceID string            `json:"instance_id"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// GetFullMetadata returns all metadata as a JSON response (custom endpoint for testing)
func (m *GCPMetadataServer) GetFullMetadata() GCPMetadataResponse {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return GCPMetadataResponse{
		ProjectID:  m.projectID,
		Zone:       m.zone,
		InstanceID: m.instanceID,
		Attributes: m.attributes,
	}
}

// ServeHTTP implements http.Handler for custom handling
func (m *GCPMetadataServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Custom endpoint for full metadata dump (for testing only)
	if r.URL.Path == "/test/full-metadata" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(m.GetFullMetadata())
		return
	}
}
