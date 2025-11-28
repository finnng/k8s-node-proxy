package mocks

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"
)

// STSAPI mocks the AWS STS (Security Token Service) API
// Used for testing IAM authentication and token generation
type STSAPI struct {
	server      *httptest.Server
	mu          sync.RWMutex
	account     string
	userID      string
	arn         string
	shouldFail  bool
	failureCode int
}

// GetCallerIdentityResponse represents the response from GetCallerIdentity
type GetCallerIdentityResponse struct {
	GetCallerIdentityResult GetCallerIdentityResult `json:"GetCallerIdentityResult"`
	ResponseMetadata        ResponseMetadata        `json:"ResponseMetadata"`
}

// GetCallerIdentityResult contains the identity information
type GetCallerIdentityResult struct {
	Account string `json:"Account"`
	UserID  string `json:"UserId"`
	ARN     string `json:"Arn"`
}

// ResponseMetadata contains request metadata
type ResponseMetadata struct {
	RequestID string `json:"RequestId"`
}

// AssumeRoleResponse represents the response from AssumeRole
type AssumeRoleResponse struct {
	AssumeRoleResult AssumeRoleResult `json:"AssumeRoleResult"`
	ResponseMetadata ResponseMetadata `json:"ResponseMetadata"`
}

// AssumeRoleResult contains the assumed role credentials
type AssumeRoleResult struct {
	Credentials      *Credentials     `json:"Credentials"`
	AssumedRoleUser  *AssumedRoleUser `json:"AssumedRoleUser"`
	PackedPolicySize int              `json:"PackedPolicySize,omitempty"`
}

// Credentials contains temporary security credentials
type Credentials struct {
	AccessKeyID     string    `json:"AccessKeyId"`
	SecretAccessKey string    `json:"SecretAccessKey"`
	SessionToken    string    `json:"SessionToken"`
	Expiration      time.Time `json:"Expiration"`
}

// AssumedRoleUser contains information about the assumed role user
type AssumedRoleUser struct {
	AssumedRoleID string `json:"AssumedRoleId"`
	ARN           string `json:"Arn"`
}

// STSErrorResponse represents an error response from STS
type STSErrorResponse struct {
	Error struct {
		Type    string `json:"Type"`
		Code    string `json:"Code"`
		Message string `json:"Message"`
	} `json:"Error"`
	RequestID string `json:"RequestId"`
}

// NewSTSAPI creates a new mock STS API server
func NewSTSAPI() *STSAPI {
	mock := &STSAPI{
		account: "123456789012",
		userID:  "AIDACKCEVSQ6C2EXAMPLE",
		arn:     "arn:aws:iam::123456789012:user/test-user",
	}

	mux := http.NewServeMux()

	// STS uses query parameters for action routing
	mux.HandleFunc("/", mock.handleRequest)

	mock.server = httptest.NewServer(mux)
	return mock
}

// URL returns the base URL of the mock server
func (m *STSAPI) URL() string {
	return m.server.URL
}

// Close shuts down the mock server
func (m *STSAPI) Close() {
	m.server.Close()
}

// SetAccount sets the AWS account ID
func (m *STSAPI) SetAccount(account string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.account = account
}

// SetUserID sets the user ID
func (m *STSAPI) SetUserID(userID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.userID = userID
}

// SetARN sets the ARN
func (m *STSAPI) SetARN(arn string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.arn = arn
}

// SetShouldFail configures the API to return errors
func (m *STSAPI) SetShouldFail(fail bool, code int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shouldFail = fail
	m.failureCode = code
}

// handleRequest routes requests based on the Action parameter
func (m *STSAPI) handleRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.shouldFail {
		m.writeErrorResponse(w, m.failureCode, "ServiceUnavailable", "STS service is temporarily unavailable")
		return
	}

	// Parse query parameters or form data
	if err := r.ParseForm(); err != nil {
		m.writeErrorResponse(w, http.StatusBadRequest, "InvalidRequest", "Failed to parse request")
		return
	}

	action := r.Form.Get("Action")
	if action == "" {
		m.writeErrorResponse(w, http.StatusBadRequest, "MissingAction", "Action parameter is required")
		return
	}

	switch action {
	case "GetCallerIdentity":
		m.handleGetCallerIdentity(w, r)
	case "AssumeRole":
		m.handleAssumeRole(w, r)
	default:
		m.writeErrorResponse(w, http.StatusBadRequest, "InvalidAction", fmt.Sprintf("Unknown action: %s", action))
	}
}

// handleGetCallerIdentity handles the GetCallerIdentity API call
func (m *STSAPI) handleGetCallerIdentity(w http.ResponseWriter, r *http.Request) {
	response := GetCallerIdentityResponse{
		GetCallerIdentityResult: GetCallerIdentityResult{
			Account: m.account,
			UserID:  m.userID,
			ARN:     m.arn,
		},
		ResponseMetadata: ResponseMetadata{
			RequestID: generateRequestID(),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// handleAssumeRole handles the AssumeRole API call
func (m *STSAPI) handleAssumeRole(w http.ResponseWriter, r *http.Request) {
	roleArn := r.Form.Get("RoleArn")
	roleSessionName := r.Form.Get("RoleSessionName")

	if roleArn == "" {
		m.writeErrorResponse(w, http.StatusBadRequest, "MissingParameter", "RoleArn parameter is required")
		return
	}

	if roleSessionName == "" {
		m.writeErrorResponse(w, http.StatusBadRequest, "MissingParameter", "RoleSessionName parameter is required")
		return
	}

	response := AssumeRoleResponse{
		AssumeRoleResult: AssumeRoleResult{
			Credentials: &Credentials{
				AccessKeyID:     "ASIAMOCKASSUMEDKEY123",
				SecretAccessKey: "mockAssumedSecretKey1234567890abcdefghij",
				SessionToken:    "mockAssumedSessionToken1234567890abcdefghijklmnopqrstuvwxyz",
				Expiration:      time.Now().Add(1 * time.Hour),
			},
			AssumedRoleUser: &AssumedRoleUser{
				AssumedRoleID: fmt.Sprintf("%s:%s", m.userID, roleSessionName),
				ARN:           fmt.Sprintf("%s/assumed-role/%s", roleArn, roleSessionName),
			},
		},
		ResponseMetadata: ResponseMetadata{
			RequestID: generateRequestID(),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// writeErrorResponse writes an STS-formatted error response
func (m *STSAPI) writeErrorResponse(w http.ResponseWriter, statusCode int, errorCode, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	errorResp := STSErrorResponse{
		RequestID: generateRequestID(),
	}
	errorResp.Error.Type = "Sender"
	errorResp.Error.Code = errorCode
	errorResp.Error.Message = message

	json.NewEncoder(w).Encode(errorResp)
}

// generateRequestID generates a mock AWS request ID
func generateRequestID() string {
	return fmt.Sprintf("mock-request-%d", time.Now().UnixNano())
}

// GetPresignedURL simulates creating a pre-signed URL for GetCallerIdentity
// This is used by AWS IAM Authenticator for EKS authentication
func GetPresignedURL(stsEndpoint, region string) string {
	// In real implementation, this would be a properly signed URL
	// For mocking, we return a simple URL that contains the necessary parameters
	return fmt.Sprintf("%s/?Action=GetCallerIdentity&Version=2011-06-15&X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Date=%s&X-Amz-SignedHeaders=host&X-Amz-Expires=60&X-Amz-Credential=mock-access-key/%s/%s/sts/aws4_request",
		stsEndpoint,
		time.Now().Format("20060102T150405Z"),
		time.Now().Format("20060102"),
		region,
	)
}
