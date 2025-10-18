package mocks

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
)

// GKEClusterResponse represents a GKE cluster API response
type GKEClusterResponse struct {
	Clusters []GKECluster `json:"clusters"`
}

// GKECluster represents a single GKE cluster
type GKECluster struct {
	Name                 string           `json:"name"`
	Location             string           `json:"location"`
	Endpoint             string           `json:"endpoint"`
	MasterAuth           GKEMasterAuth    `json:"masterAuth"`
	PrivateClusterConfig GKEPrivateConfig `json:"privateClusterConfig"`
	Status               string           `json:"status"`
}

// GKEMasterAuth contains cluster authentication info
type GKEMasterAuth struct {
	ClusterCaCertificate string `json:"clusterCaCertificate"`
}

// GKEPrivateConfig contains private endpoint configuration
type GKEPrivateConfig struct {
	PrivateEndpoint string `json:"privateEndpoint"`
	PublicEndpoint  string `json:"publicEndpoint"`
}

// NewMockGKEContainerAPI creates a test HTTP server that mocks GKE Container API
func NewMockGKEContainerAPI(clusters []GKECluster) *httptest.Server {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Match the GKE API endpoint pattern: /v1/projects/{project}/locations/-/clusters
		if r.Method == "GET" && containsPath(r.URL.Path, "/clusters") {
			w.Header().Set("Content-Type", "application/json")
			response := GKEClusterResponse{
				Clusters: clusters,
			}
			json.NewEncoder(w).Encode(response)
			return
		}

		// Default response for unmatched endpoints
		http.NotFound(w, r)
	})

	return httptest.NewServer(handler)
}

// NewDefaultMockGKECluster creates a mock GKE cluster with sensible defaults
func NewDefaultMockGKECluster() GKECluster {
	// Base64 encoded dummy CA certificate (valid format but not a real cert)
	// This is just "test-ca-certificate" base64 encoded
	dummyCACert := "dGVzdC1jYS1jZXJ0aWZpY2F0ZQ=="

	return GKECluster{
		Name:     "test-gke-cluster",
		Location: "us-central1",
		Endpoint: "34.123.45.67",
		MasterAuth: GKEMasterAuth{
			ClusterCaCertificate: dummyCACert,
		},
		PrivateClusterConfig: GKEPrivateConfig{
			PrivateEndpoint: "10.0.0.1",
			PublicEndpoint:  "34.123.45.67",
		},
		Status: "RUNNING",
	}
}

// containsPath checks if a URL path contains a substring
func containsPath(path, substr string) bool {
	return strings.Contains(path, substr)
}
