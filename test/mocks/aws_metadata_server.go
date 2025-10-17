package mocks

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"
)

// AWSMetadataServer mocks the AWS EC2 Instance Metadata Service (IMDSv2)
// Implements IMDSv2 token-based authentication flow
type AWSMetadataServer struct {
	server           *httptest.Server
	mu               sync.RWMutex
	region           string
	instanceID       string
	instanceType     string
	availabilityZone string
	iamRole          string
	tokens           map[string]time.Time // token -> expiry
	shouldFail       bool
	failureCode      int
	tokenTTL         time.Duration
}

// NewAWSMetadataServer creates a new mock AWS metadata server with IMDSv2 support
func NewAWSMetadataServer() *AWSMetadataServer {
	mock := &AWSMetadataServer{
		region:           "us-east-1",
		instanceID:       "i-1234567890abcdef0",
		instanceType:     "t3.medium",
		availabilityZone: "us-east-1a",
		iamRole:          "test-eks-node-role",
		tokens:           make(map[string]time.Time),
		tokenTTL:         6 * time.Hour, // Default IMDSv2 token TTL
	}

	mux := http.NewServeMux()

	// IMDSv2 token endpoint
	mux.HandleFunc("/latest/api/token", mock.handleTokenRequest)

	// Metadata endpoints (require IMDSv2 token)
	mux.HandleFunc("/latest/meta-data/instance-id", mock.handleInstanceID)
	mux.HandleFunc("/latest/meta-data/instance-type", mock.handleInstanceType)
	mux.HandleFunc("/latest/meta-data/placement/availability-zone", mock.handleAvailabilityZone)
	mux.HandleFunc("/latest/meta-data/placement/region", mock.handleRegion)
	mux.HandleFunc("/latest/meta-data/iam/security-credentials/", mock.handleIAMRoleList)
	mux.HandleFunc("/latest/meta-data/iam/security-credentials/"+mock.iamRole, mock.handleIAMCredentials)

	// Dynamic data
	mux.HandleFunc("/latest/dynamic/instance-identity/document", mock.handleInstanceIdentityDocument)

	mock.server = httptest.NewServer(mux)
	return mock
}

// URL returns the base URL of the mock server
func (m *AWSMetadataServer) URL() string {
	return m.server.URL
}

// Close shuts down the mock server
func (m *AWSMetadataServer) Close() {
	m.server.Close()
}

// SetRegion sets the AWS region to be returned
func (m *AWSMetadataServer) SetRegion(region string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.region = region
}

// SetInstanceID sets the instance ID
func (m *AWSMetadataServer) SetInstanceID(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.instanceID = id
}

// SetIAMRole sets the IAM role name
func (m *AWSMetadataServer) SetIAMRole(role string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.iamRole = role
}

// SetShouldFail configures the server to return errors
func (m *AWSMetadataServer) SetShouldFail(fail bool, code int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shouldFail = fail
	m.failureCode = code
}

// handleTokenRequest handles IMDSv2 token generation
// PUT /latest/api/token
// Header: X-aws-ec2-metadata-token-ttl-seconds: 21600
func (m *AWSMetadataServer) handleTokenRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.shouldFail {
		http.Error(w, "token generation failed", m.failureCode)
		return
	}

	// Parse TTL from header
	ttlHeader := r.Header.Get("X-aws-ec2-metadata-token-ttl-seconds")
	if ttlHeader == "" {
		http.Error(w, "Missing X-aws-ec2-metadata-token-ttl-seconds header", http.StatusBadRequest)
		return
	}

	// Generate a simple token (in real IMDS, this would be a secure token)
	token := fmt.Sprintf("mock-token-%d", time.Now().UnixNano())

	m.mu.RUnlock()
	m.mu.Lock()
	m.tokens[token] = time.Now().Add(m.tokenTTL)
	m.mu.Unlock()
	m.mu.RLock()

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(token))
}

// validateToken checks if the provided IMDSv2 token is valid
func (m *AWSMetadataServer) validateToken(r *http.Request) bool {
	token := r.Header.Get("X-aws-ec2-metadata-token")
	if token == "" {
		return false
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	expiry, exists := m.tokens[token]
	if !exists {
		return false
	}

	return time.Now().Before(expiry)
}

// handleInstanceID handles the instance ID metadata endpoint
func (m *AWSMetadataServer) handleInstanceID(w http.ResponseWriter, r *http.Request) {
	if !m.validateToken(r) {
		http.Error(w, "Unauthorized - Invalid or missing IMDSv2 token", http.StatusUnauthorized)
		return
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.shouldFail {
		http.Error(w, "metadata server error", m.failureCode)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(m.instanceID))
}

// handleInstanceType handles the instance type metadata endpoint
func (m *AWSMetadataServer) handleInstanceType(w http.ResponseWriter, r *http.Request) {
	if !m.validateToken(r) {
		http.Error(w, "Unauthorized - Invalid or missing IMDSv2 token", http.StatusUnauthorized)
		return
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(m.instanceType))
}

// handleAvailabilityZone handles the availability zone metadata endpoint
func (m *AWSMetadataServer) handleAvailabilityZone(w http.ResponseWriter, r *http.Request) {
	if !m.validateToken(r) {
		http.Error(w, "Unauthorized - Invalid or missing IMDSv2 token", http.StatusUnauthorized)
		return
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(m.availabilityZone))
}

// handleRegion handles the region metadata endpoint
func (m *AWSMetadataServer) handleRegion(w http.ResponseWriter, r *http.Request) {
	if !m.validateToken(r) {
		http.Error(w, "Unauthorized - Invalid or missing IMDSv2 token", http.StatusUnauthorized)
		return
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(m.region))
}

// handleIAMRoleList lists available IAM roles
func (m *AWSMetadataServer) handleIAMRoleList(w http.ResponseWriter, r *http.Request) {
	if !m.validateToken(r) {
		http.Error(w, "Unauthorized - Invalid or missing IMDSv2 token", http.StatusUnauthorized)
		return
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(m.iamRole + "\n"))
}

// IAMCredentials represents temporary IAM credentials
type IAMCredentials struct {
	Code            string    `json:"Code"`
	LastUpdated     time.Time `json:"LastUpdated"`
	Type            string    `json:"Type"`
	AccessKeyID     string    `json:"AccessKeyId"`
	SecretAccessKey string    `json:"SecretAccessKey"`
	Token           string    `json:"Token"`
	Expiration      time.Time `json:"Expiration"`
}

// handleIAMCredentials returns mock IAM credentials
func (m *AWSMetadataServer) handleIAMCredentials(w http.ResponseWriter, r *http.Request) {
	if !m.validateToken(r) {
		http.Error(w, "Unauthorized - Invalid or missing IMDSv2 token", http.StatusUnauthorized)
		return
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.shouldFail {
		http.Error(w, "metadata server error", m.failureCode)
		return
	}

	credentials := IAMCredentials{
		Code:            "Success",
		LastUpdated:     time.Now(),
		Type:            "AWS-HMAC",
		AccessKeyID:     "ASIAMOCKKEY123456789",
		SecretAccessKey: "mockSecretAccessKey1234567890abcdefghij",
		Token:           "mockSessionToken1234567890abcdefghijklmnopqrstuvwxyz",
		Expiration:      time.Now().Add(6 * time.Hour),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(credentials)
}

// InstanceIdentityDocument represents the instance identity document
type InstanceIdentityDocument struct {
	AccountID        string    `json:"accountId"`
	Architecture     string    `json:"architecture"`
	AvailabilityZone string    `json:"availabilityZone"`
	ImageID          string    `json:"imageId"`
	InstanceID       string    `json:"instanceId"`
	InstanceType     string    `json:"instanceType"`
	PrivateIP        string    `json:"privateIp"`
	Region           string    `json:"region"`
	Version          string    `json:"version"`
	PendingTime      time.Time `json:"pendingTime"`
}

// handleInstanceIdentityDocument returns the instance identity document
func (m *AWSMetadataServer) handleInstanceIdentityDocument(w http.ResponseWriter, r *http.Request) {
	if !m.validateToken(r) {
		http.Error(w, "Unauthorized - Invalid or missing IMDSv2 token", http.StatusUnauthorized)
		return
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.shouldFail {
		http.Error(w, "metadata server error", m.failureCode)
		return
	}

	doc := InstanceIdentityDocument{
		AccountID:        "123456789012",
		Architecture:     "x86_64",
		AvailabilityZone: m.availabilityZone,
		ImageID:          "ami-0c55b159cbfafe1f0",
		InstanceID:       m.instanceID,
		InstanceType:     m.instanceType,
		PrivateIP:        "10.0.1.100",
		Region:           m.region,
		Version:          "2017-09-30",
		PendingTime:      time.Now().Add(-24 * time.Hour),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(doc)
}

// CleanupExpiredTokens removes expired tokens from the token map
func (m *AWSMetadataServer) CleanupExpiredTokens() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for token, expiry := range m.tokens {
		if now.After(expiry) {
			delete(m.tokens, token)
		}
	}
}
