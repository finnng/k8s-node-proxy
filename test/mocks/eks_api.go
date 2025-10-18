package mocks

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"
)

// EKS API mocks the AWS EKS API for testing
// Used for testing cluster discovery and configuration retrieval
type EKSAPI struct {
	server      *httptest.Server
	mu          sync.RWMutex
	clusters    map[string]*EKSCluster // key: cluster name
	shouldFail  bool
	failureCode int
}

// EKSCluster represents an EKS cluster
type EKSCluster struct {
	Name                 string                `json:"name"`
	ARN                  string                `json:"arn"`
	CreatedAt            time.Time             `json:"createdAt"`
	Version              string                `json:"version"`
	Endpoint             string                `json:"endpoint"`
	ResourcesVPCConfig   *VPCConfigResponse    `json:"resourcesVpcConfig"`
	CertificateAuthority *CertificateAuthority `json:"certificateAuthority"`
	Status               string                `json:"status"`
	Tags                 map[string]string     `json:"tags,omitempty"`
}

// VPCConfigResponse contains VPC configuration for the cluster
type VPCConfigResponse struct {
	SubnetIDs              []string `json:"subnetIds"`
	SecurityGroupIDs       []string `json:"securityGroupIds"`
	ClusterSecurityGroupID string   `json:"clusterSecurityGroupId"`
	VPCID                  string   `json:"vpcId"`
	EndpointPublicAccess   bool     `json:"endpointPublicAccess"`
	EndpointPrivateAccess  bool     `json:"endpointPrivateAccess"`
	PublicAccessCIDRs      []string `json:"publicAccessCidrs"`
}

// CertificateAuthority contains the cluster CA certificate
type CertificateAuthority struct {
	Data string `json:"data"` // Base64 encoded certificate
}

// DescribeClusterResponse is the response from DescribeCluster API
type DescribeClusterResponse struct {
	Cluster *EKSCluster `json:"cluster"`
}

// ListClustersResponse is the response from ListClusters API
type ListClustersResponse struct {
	Clusters  []string `json:"clusters"` // List of cluster names
	NextToken string   `json:"nextToken,omitempty"`
}

// NewEKSAPI creates a new mock EKS API server
func NewEKSAPI() *EKSAPI {
	mock := &EKSAPI{
		clusters: make(map[string]*EKSCluster),
	}

	// Add default test cluster
	mock.addDefaultCluster()

	mux := http.NewServeMux()

	// EKS API endpoints
	mux.HandleFunc("/clusters", mock.handleListClusters)
	mux.HandleFunc("/clusters/", mock.handleDescribeCluster)

	mock.server = httptest.NewServer(mux)
	return mock
}

// URL returns the base URL of the mock server
func (m *EKSAPI) URL() string {
	return m.server.URL
}

// Close shuts down the mock server
func (m *EKSAPI) Close() {
	m.server.Close()
}

// AddCluster adds a cluster to the mock API
func (m *EKSAPI) AddCluster(cluster *EKSCluster) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clusters[cluster.Name] = cluster
}

// SetShouldFail configures the API to return errors
func (m *EKSAPI) SetShouldFail(fail bool, code int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shouldFail = fail
	m.failureCode = code
}

// addDefaultCluster creates a default test cluster
func (m *EKSAPI) addDefaultCluster() {
	cluster := &EKSCluster{
		Name:      "test-eks-cluster",
		ARN:       "arn:aws:eks:us-east-1:123456789012:cluster/test-eks-cluster",
		CreatedAt: time.Now().Add(-30 * 24 * time.Hour),
		Version:   "1.28",
		Endpoint:  "https://ABC123DEF456.gr7.us-east-1.eks.amazonaws.com",
		ResourcesVPCConfig: &VPCConfigResponse{
			SubnetIDs:              []string{"subnet-12345", "subnet-67890"},
			SecurityGroupIDs:       []string{"sg-12345"},
			ClusterSecurityGroupID: "sg-cluster-12345",
			VPCID:                  "vpc-12345",
			EndpointPublicAccess:   false,
			EndpointPrivateAccess:  true,
			PublicAccessCIDRs:      []string{},
		},
		CertificateAuthority: &CertificateAuthority{
			Data: "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURJVENDQWdtZ0F3SUJBZ0lSQUtPRWF0UGF6RDN5VjJmTWdxTzZTWU13RFFZSktvWklodmNOQVFFTEJRQXcKRlRFVE1CRUdBMVVFQXhNS2EzVmlaWEp1WlhSbGN6QWVGdzB5TkRFeE1qQXhOalF3TlRCYUZ3MHpOREV4TVRneApOalF3TlRCYU1CVXhFekFSQmdOVkJBTVRDbXQxWW1WeWJtVjBaWE13Z2dFaU1BMEdDU3FHU0liM0RRRUJBUVVBCkE0SUJEd0F3Z2dFS0FvSUJBUUN3Tk9Fb1F1Y3hEZkJnU2lyMWRHYWxhVU9KQ1Z3RVd1ME15eHRROGM1aDZ4cFoKdzhCNXpXa1FEYmVFZGZaa1E5c1hoeFVaMnowdUw5NVhEWEF3VEY5YS9rYjRaTDVkU1JwNXhDZ3ljK0pWOE5DVgpXWGNXelBCUjhVb3c0MnZGVVo5bHQzdmFLcnhNSDdvZEJ3dXJ4eUhVWU9wRHFGdCtad0pEeEUyZFRmdzBKN3NCCklGV2xGWG1WV3VMNDdaZ2lZdXFSR1piR1l4d2VZSHdQdUErS1lvQ1RwWW8vYVRMUGJTWDRocG5xN1lPVnk5Y00KTE5IOUhmeU9iVXNic0l1RnJUQ0RrWDQvS1hsbk9HQUpxUmxlYWI1WGExT0crSGl1cTRpMXVwT0pnU05LSzVKeQpqMnFYNWFsQ2dtQnpid2hueUF2OEFEd2pqMm9zSTExWVBJQkRBZ01CQUFHalZ6QlZNQTRHQTFVZER3RUIvd1FFCkF3SUNwREFQQmdOVkhSTUJBZjhFQlRBREFRSC9NQjBHQTFVZERnUVdCQlR0OGlzUjBWZEVsbU1hcDNiQmY1L1YKYWt5T0RqQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFOQmdrcWhraUc5dzBCQVFzRkFBT0NBUUVBaE9hZgoxNzM5dGdDaVhxU1d2cVpZb2tUdHI0THg5c3JDR3VwRG1ZZHpGZzdEazJJdmIzU1AreTJwMzhRYXBVN1JSdXduCmR4V3FVT3RPcWRDUHErOFhHTWZJTm15aW04YlJLRW5pOEd0RGF4TFN1V1RKWWNZR0IxSEFja2RrOUttQ1IrOEgKeFh2Q0RtNVNwMTlwSEZQZGttNnRLZGJlSGE2cEk5d3ZYV2U3cWp0QjNnRjZRMUhDcDB1c3hYeU1meW1leVhXOAp1NGRGcmZHbE5ZL25pWGxrSW9ZaGxaY2dKR1VoeHZSeEdkZ3NTRzhHdlRRSWlmYUQvUkxabGdFakJkYkdJTmN6Cm1NQ1RWQkFYekNqVUJBSWg2MVhMcGlWZDBJM1N5TDltMHEvZy9mNXFPbjRIdGtxUGNXdXA4OGlPZ1R4TXhJMVQKRHVwbXlLYm1jcFBiWlE9PQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg==",
		},
		Status: "ACTIVE",
		Tags: map[string]string{
			"Environment": "test",
			"ManagedBy":   "terraform",
		},
	}

	m.AddCluster(cluster)
}

// handleListClusters handles the ListClusters API call
// GET /clusters
func (m *EKSAPI) handleListClusters(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.shouldFail {
		m.writeErrorResponse(w, m.failureCode, "ServiceUnavailable", "EKS service is unavailable")
		return
	}

	var clusterNames []string
	for name := range m.clusters {
		clusterNames = append(clusterNames, name)
	}

	response := ListClustersResponse{
		Clusters: clusterNames,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// handleDescribeCluster handles the DescribeCluster API call
// GET /clusters/{name}
func (m *EKSAPI) handleDescribeCluster(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.shouldFail {
		m.writeErrorResponse(w, m.failureCode, "ServiceUnavailable", "EKS service is unavailable")
		return
	}

	// Extract cluster name from path: /clusters/{name}
	clusterName := r.URL.Path[len("/clusters/"):]
	if clusterName == "" {
		m.writeErrorResponse(w, http.StatusBadRequest, "InvalidParameterException", "Cluster name is required")
		return
	}

	cluster, exists := m.clusters[clusterName]
	if !exists {
		m.writeErrorResponse(w, http.StatusNotFound, "ResourceNotFoundException", fmt.Sprintf("No cluster found for name: %s", clusterName))
		return
	}

	response := DescribeClusterResponse{
		Cluster: cluster,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// EKSErrorResponse represents an error response from EKS API
type EKSErrorResponse struct {
	Type    string `json:"__type"`
	Message string `json:"message"`
}

// writeErrorResponse writes an EKS-formatted error response
func (m *EKSAPI) writeErrorResponse(w http.ResponseWriter, statusCode int, errorType, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	errorResp := EKSErrorResponse{
		Type:    errorType,
		Message: message,
	}

	json.NewEncoder(w).Encode(errorResp)
}

// CreateTestCluster creates a test EKS cluster with custom configuration
func CreateTestCluster(name, region, endpoint string) *EKSCluster {
	return &EKSCluster{
		Name:      name,
		ARN:       fmt.Sprintf("arn:aws:eks:%s:123456789012:cluster/%s", region, name),
		CreatedAt: time.Now().Add(-30 * 24 * time.Hour),
		Version:   "1.28",
		Endpoint:  endpoint,
		ResourcesVPCConfig: &VPCConfigResponse{
			SubnetIDs:              []string{"subnet-test1", "subnet-test2"},
			SecurityGroupIDs:       []string{"sg-test"},
			ClusterSecurityGroupID: "sg-cluster-test",
			VPCID:                  "vpc-test",
			EndpointPublicAccess:   false,
			EndpointPrivateAccess:  true,
		},
		CertificateAuthority: &CertificateAuthority{
			Data: "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURJVENDQWdtZ0F3SUJBZ0lSQUtPRWF0UGF6RDN5VjJmTWdxTzZTWU13RFFZSktvWklodmNOQVFFTEJRQXcKRlRFVE1CRUdBMVVFQXhNS2EzVmlaWEp1WlhSbGN6QWVGdzB5TkRFeE1qQXhOalF3TlRCYUZ3MHpOREV4TVRneApOalF3TlRCYU1CVXhFekFSQmdOVkJBTVRDbXQxWW1WeWJtVjBaWE13Z2dFaU1BMEdDU3FHU0liM0RRRUJBUVVBCkE0SUJEd0F3Z2dFS0FvSUJBUUN3Tk9Fb1F1Y3hEZkJnU2lyMWRHYWxhVU9KQ1Z3RVd1ME15eHRROGM1aDZ4cFoKdzhCNXpXa1FEYmVFZGZaa1E5c1hoeFVaMnowdUw5NVhEWEF3VEY5YS9rYjRaTDVkU1JwNXhDZ3ljK0pWOE5DVgpXWGNXelBCUjhVb3c0MnZGVVo5bHQzdmFLcnhNSDdvZEJ3dXJ4eUhVWU9wRHFGdCtad0pEeEUyZFRmdzBKN3NCCklGV2xGWG1WV3VMNDdaZ2lZdXFSR1piR1l4d2VZSHdQdUErS1lvQ1RwWW8vYVRMUGJTWDRocG5xN1lPVnk5Y00KTE5IOUhmeU9iVXNic0l1RnJUQ0RrWDQvS1hsbk9HQUpxUmxlYWI1WGExT0crSGl1cTRpMXVwT0pnU05LSzVKeQpqMnFYNWFsQ2dtQnpid2hueUF2OEFEd2pqMm9zSTExWVBJQkRBZ01CQUFHalZ6QlZNQTRHQTFVZER3RUIvd1FFCkF3SUNwREFQQmdOVkhSTUJBZjhFQlRBREFRSC9NQjBHQTFVZERnUVdCQlR0OGlzUjBWZEVsbU1hcDNiQmY1L1YKYWt5T0RqQVRCZ05WSFNVRUREQUtCZ2dyQmdFRkJRY0RBVEFOQmdrcWhraUc5dzBCQVFzRkFBT0NBUUVBaE9hZgoxNzM5dGdDaVhxU1d2cVpZb2tUdHI0THg5c3JDR3VwRG1ZZHpGZzdEazJJdmIzU1AreTJwMzhRYXBVN1JSdXduCmR4V3FVT3RPcWRDUHErOFhHTWZJTm15aW04YlJLRW5pOEd0RGF4TFN1V1RKWWNZR0IxSEFja2RrOUttQ1IrOEgKeFh2Q0RtNVNwMTlwSEZQZGttNnRLZGJlSGE2cEk5d3ZYV2U3cWp0QjNnRjZRMUhDcDB1c3hYeU1meW1leVhXOAp1NGRGcmZHbE5ZL25pWGxrSW9ZaGxaY2dKR1VoeHZSeEdkZ3NTRzhHdlRRSWlmYUQvUkxabGdFakJkYkdJTmN6Cm1NQ1RWQkFYekNqVUJBSWg2MVhMcGlWZDBJM1N5TDltMHEvZy9mNXFPbjRIdGtxUGNXdXA4OGlPZ1R4TXhJMVQKRHVwbXlLYm1jcFBiWlE9PQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg==",
		},
		Status: "ACTIVE",
	}
}
