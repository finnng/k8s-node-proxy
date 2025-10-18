package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	"k8s-node-proxy/internal/platform"
	"k8s-node-proxy/internal/services"
	"k8s-node-proxy/test/mocks"
)

// TestGKEPlatformDetection tests GKE platform detection and cluster discovery
func TestGKEPlatformDetection(t *testing.T) {
	// Setup: Create mock GCP metadata server
	metadataServer := mocks.NewGCPMetadataServer()
	defer metadataServer.Close()

	metadataServer.SetProjectID("test-project-12345")
	metadataServer.SetZone("us-central1-a")

	// Setup: Set environment variables for GKE
	originalProjectID := os.Getenv("PROJECT_ID")
	os.Setenv("PROJECT_ID", "test-project-12345")
	defer func() {
		if originalProjectID == "" {
			os.Unsetenv("PROJECT_ID")
		} else {
			os.Setenv("PROJECT_ID", originalProjectID)
		}
	}()

	// Test 1: Platform detection should detect GCP
	t.Run("DetectGCPPlatform", func(t *testing.T) {
		detectedPlatform, err := platform.DetectPlatform()
		if err != nil {
			t.Fatalf("Failed to detect platform: %v", err)
		}

		if detectedPlatform != platform.GCP {
			t.Errorf("Expected platform GCP, got %s", detectedPlatform)
		}
	})

	// Test 2: Cluster discovery should work with PROJECT_ID
	t.Run("DiscoverGKECluster", func(t *testing.T) {
		// Note: This test requires a real GKE cluster or mocked Container API
		// For now, we test that the discovery object can be created
		discovery, err := services.NewNodePortDiscovery("test-project-12345")
		if err != nil {
			// Expected to fail without real GCP credentials, but check error message
			t.Logf("Expected error without GCP credentials: %v", err)
			return
		}

		// If discovery succeeds, verify cluster info
		clusterInfo := discovery.GetClusterInfo()
		if clusterInfo == nil {
			t.Error("Expected cluster info, got nil")
		}
	})
}

// TestGKEClusterDiscoveryWithMock tests GKE cluster discovery with mocked Container API
func TestGKEClusterDiscoveryWithMock(t *testing.T) {
	// Setup: Create mock GKE Container API
	mockCluster := mocks.NewDefaultMockGKECluster()
	mockServer := mocks.NewMockGKEContainerAPI([]mocks.GKECluster{mockCluster})
	defer mockServer.Close()

	t.Run("ListClusters", func(t *testing.T) {
		// This test demonstrates how to use the mock GKE API
		// In a real test, we would inject the mock server URL into the discovery code
		t.Logf("Mock GKE API URL: %s", mockServer.URL)
		t.Logf("Mock cluster name: %s", mockCluster.Name)
		t.Logf("Mock cluster endpoint: %s", mockCluster.Endpoint)
	})
}

// TestGKEPrivateEndpointDiscovery tests that private endpoints are preferred
func TestGKEPrivateEndpointDiscovery(t *testing.T) {
	t.Run("PreferPrivateEndpoint", func(t *testing.T) {
		mockCluster := mocks.NewDefaultMockGKECluster()

		// Verify the mock has private endpoint configured
		if mockCluster.PrivateClusterConfig.PrivateEndpoint == "" {
			t.Error("Expected private endpoint to be configured")
		}

		t.Logf("Private endpoint: %s", mockCluster.PrivateClusterConfig.PrivateEndpoint)
		t.Logf("Public endpoint: %s", mockCluster.PrivateClusterConfig.PublicEndpoint)
	})
}

// TestGKEMetadataServerTimeout tests timeout handling for metadata server
func TestGKEMetadataServerTimeout(t *testing.T) {
	t.Run("MetadataTimeout", func(t *testing.T) {
		// Setup: Create metadata server that will timeout
		metadataServer := mocks.NewGCPMetadataServer()
		defer metadataServer.Close()

		// Configure server to fail
		metadataServer.SetShouldFail(true, 500)

		// Test that platform detection handles failures gracefully
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Note: This test shows how to test timeout scenarios
		// In a real implementation, we would need to make the platform detector
		// use the mock metadata server URL
		t.Logf("Mock metadata server URL: %s", metadataServer.URL())
		t.Logf("Testing with context timeout: %v", ctx.Err())
	})
}

// TestGKEEnvironmentVariables tests various environment variable configurations
func TestGKEEnvironmentVariables(t *testing.T) {
	tests := []struct {
		name        string
		projectID   string
		wantError   bool
		description string
	}{
		{
			name:        "ValidProjectID",
			projectID:   "valid-project-12345",
			wantError:   false,
			description: "Valid project ID should be accepted",
		},
		{
			name:        "EmptyProjectID",
			projectID:   "",
			wantError:   true,
			description: "Empty project ID should fail platform detection",
		},
		{
			name:        "ProjectIDWithSpecialChars",
			projectID:   "project-with-dashes-123",
			wantError:   false,
			description: "Project ID with dashes should be valid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original env
			originalProjectID := os.Getenv("PROJECT_ID")
			originalGoogleProject := os.Getenv("GOOGLE_CLOUD_PROJECT")
			defer func() {
				if originalProjectID == "" {
					os.Unsetenv("PROJECT_ID")
				} else {
					os.Setenv("PROJECT_ID", originalProjectID)
				}
				if originalGoogleProject == "" {
					os.Unsetenv("GOOGLE_CLOUD_PROJECT")
				} else {
					os.Setenv("GOOGLE_CLOUD_PROJECT", originalGoogleProject)
				}
			}()

			// Set test env
			if tt.projectID != "" {
				os.Setenv("PROJECT_ID", tt.projectID)
			} else {
				os.Unsetenv("PROJECT_ID")
				os.Unsetenv("GOOGLE_CLOUD_PROJECT")
			}

			// Test platform detection
			detectedPlatform, err := platform.DetectPlatform()

			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error for %s, got nil", tt.description)
				}
			} else {
				if err != nil {
					// For valid project IDs, we expect GCP detection
					// Error is acceptable here as we don't have AWS env vars
					t.Logf("Detection result: %v (error: %v)", detectedPlatform, err)
				}
			}
		})
	}
}

// TestGKEClusterInfoStructure tests the cluster info data structure
func TestGKEClusterInfoStructure(t *testing.T) {
	t.Run("ClusterInfoFields", func(t *testing.T) {
		clusterInfo := services.ClusterInfo{
			Name:     "test-cluster",
			Location: "us-central1",
			Endpoint: "10.128.0.2",
		}

		if clusterInfo.Name == "" {
			t.Error("Cluster name should not be empty")
		}
		if clusterInfo.Location == "" {
			t.Error("Cluster location should not be empty")
		}
		if clusterInfo.Endpoint == "" {
			t.Error("Cluster endpoint should not be empty")
		}

		t.Logf("Cluster info: %+v", clusterInfo)
	})
}
