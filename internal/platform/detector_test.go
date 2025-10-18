package platform

import (
	"os"
	"testing"
)

// TestDetectPlatform_GCP tests GCP detection via PROJECT_ID (T014)
func TestDetectPlatform_GCP(t *testing.T) {
	// Save original env vars
	originalProjectID := os.Getenv("PROJECT_ID")
	originalGoogleProject := os.Getenv("GOOGLE_CLOUD_PROJECT")
	originalAWSRegion := os.Getenv("AWS_REGION")
	defer func() {
		restoreEnv("PROJECT_ID", originalProjectID)
		restoreEnv("GOOGLE_CLOUD_PROJECT", originalGoogleProject)
		restoreEnv("AWS_REGION", originalAWSRegion)
	}()

	tests := []struct {
		name          string
		projectID     string
		googleProject string
		awsRegion     string
		wantPlatform  Platform
		wantErr       bool
	}{
		{
			name:         "PROJECT_ID set",
			projectID:    "my-gcp-project",
			wantPlatform: GCP,
			wantErr:      false,
		},
		{
			name:          "GOOGLE_CLOUD_PROJECT set",
			googleProject: "my-gcp-project",
			wantPlatform:  GCP,
			wantErr:       false,
		},
		{
			name:         "PROJECT_ID takes precedence over AWS_REGION",
			projectID:    "my-gcp-project",
			awsRegion:    "us-west-2",
			wantPlatform: GCP,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment
			os.Unsetenv("PROJECT_ID")
			os.Unsetenv("GOOGLE_CLOUD_PROJECT")
			os.Unsetenv("AWS_REGION")

			if tt.projectID != "" {
				os.Setenv("PROJECT_ID", tt.projectID)
			}
			if tt.googleProject != "" {
				os.Setenv("GOOGLE_CLOUD_PROJECT", tt.googleProject)
			}
			if tt.awsRegion != "" {
				os.Setenv("AWS_REGION", tt.awsRegion)
			}

			// Test platform detection
			platform, err := DetectPlatform()

			if (err != nil) != tt.wantErr {
				t.Errorf("DetectPlatform() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if platform != tt.wantPlatform {
				t.Errorf("DetectPlatform() = %v, want %v", platform, tt.wantPlatform)
			}
		})
	}
}

// TestDetectPlatform_AWS tests AWS detection via AWS_REGION (T015)
func TestDetectPlatform_AWS(t *testing.T) {
	// Save original env vars
	originalProjectID := os.Getenv("PROJECT_ID")
	originalGoogleProject := os.Getenv("GOOGLE_CLOUD_PROJECT")
	originalAWSRegion := os.Getenv("AWS_REGION")
	defer func() {
		restoreEnv("PROJECT_ID", originalProjectID)
		restoreEnv("GOOGLE_CLOUD_PROJECT", originalGoogleProject)
		restoreEnv("AWS_REGION", originalAWSRegion)
	}()

	tests := []struct {
		name         string
		awsRegion    string
		wantPlatform Platform
		wantErr      bool
	}{
		{
			name:         "AWS_REGION set to us-west-2",
			awsRegion:    "us-west-2",
			wantPlatform: AWS,
			wantErr:      false,
		},
		{
			name:         "AWS_REGION set to us-east-1",
			awsRegion:    "us-east-1",
			wantPlatform: AWS,
			wantErr:      false,
		},
		{
			name:         "AWS_REGION set to eu-west-1",
			awsRegion:    "eu-west-1",
			wantPlatform: AWS,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment (clear GCP vars to ensure AWS detection)
			os.Unsetenv("PROJECT_ID")
			os.Unsetenv("GOOGLE_CLOUD_PROJECT")
			os.Unsetenv("AWS_REGION")

			if tt.awsRegion != "" {
				os.Setenv("AWS_REGION", tt.awsRegion)
			}

			// Test platform detection
			platform, err := DetectPlatform()

			if (err != nil) != tt.wantErr {
				t.Errorf("DetectPlatform() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if platform != tt.wantPlatform {
				t.Errorf("DetectPlatform() = %v, want %v", platform, tt.wantPlatform)
			}
		})
	}
}

// TestDetectPlatform_Generic tests Generic Kubernetes detection via KUBECONFIG (T017)
func TestDetectPlatform_Generic(t *testing.T) {
	// Save original env vars
	originalProjectID := os.Getenv("PROJECT_ID")
	originalGoogleProject := os.Getenv("GOOGLE_CLOUD_PROJECT")
	originalAWSRegion := os.Getenv("AWS_REGION")
	originalKubeconfig := os.Getenv("KUBECONFIG")
	originalK8SEndpoint := os.Getenv("K8S_ENDPOINT")
	originalK8SToken := os.Getenv("K8S_TOKEN")
	originalK8SCACert := os.Getenv("K8S_CA_CERT")
	defer func() {
		restoreEnv("PROJECT_ID", originalProjectID)
		restoreEnv("GOOGLE_CLOUD_PROJECT", originalGoogleProject)
		restoreEnv("AWS_REGION", originalAWSRegion)
		restoreEnv("KUBECONFIG", originalKubeconfig)
		restoreEnv("K8S_ENDPOINT", originalK8SEndpoint)
		restoreEnv("K8S_TOKEN", originalK8SToken)
		restoreEnv("K8S_CA_CERT", originalK8SCACert)
	}()

	tests := []struct {
		name         string
		kubeconfig   string
		k8sEndpoint  string
		k8sToken     string
		k8sCACert    string
		wantPlatform Platform
		wantErr      bool
	}{
		{
			name:         "KUBECONFIG set",
			kubeconfig:   "/path/to/kubeconfig",
			wantPlatform: Generic,
			wantErr:      false,
		},
		{
			name:         "K8S_* env vars set",
			k8sEndpoint:  "https://k8s.example.com:6443",
			k8sToken:     "eyJhbGciOiJSUzI1NiIsImtpZCI6...",
			k8sCACert:    "LS0tLS1CRUdJTi...",
			wantPlatform: Generic,
			wantErr:      false,
		},
		{
			name:         "KUBECONFIG takes precedence over K8S_* vars",
			kubeconfig:   "/path/to/kubeconfig",
			k8sEndpoint:  "https://k8s.example.com:6443",
			k8sToken:     "eyJhbGciOiJSUzI1NiIsImtpZCI6...",
			k8sCACert:    "LS0tLS1CRUdJTi...",
			wantPlatform: Generic,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment (clear other platform vars)
			os.Unsetenv("PROJECT_ID")
			os.Unsetenv("GOOGLE_CLOUD_PROJECT")
			os.Unsetenv("AWS_REGION")
			os.Unsetenv("KUBECONFIG")
			os.Unsetenv("K8S_ENDPOINT")
			os.Unsetenv("K8S_TOKEN")
			os.Unsetenv("K8S_CA_CERT")

			if tt.kubeconfig != "" {
				os.Setenv("KUBECONFIG", tt.kubeconfig)
			}
			if tt.k8sEndpoint != "" {
				os.Setenv("K8S_ENDPOINT", tt.k8sEndpoint)
			}
			if tt.k8sToken != "" {
				os.Setenv("K8S_TOKEN", tt.k8sToken)
			}
			if tt.k8sCACert != "" {
				os.Setenv("K8S_CA_CERT", tt.k8sCACert)
			}

			// Test platform detection
			platform, err := DetectPlatform()

			if (err != nil) != tt.wantErr {
				t.Errorf("DetectPlatform() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if platform != tt.wantPlatform {
				t.Errorf("DetectPlatform() = %v, want %v", platform, tt.wantPlatform)
			}
		})
	}
}

// TestDetectPlatform_Error tests error case when neither platform is detected (T016)
func TestDetectPlatform_Error(t *testing.T) {
	originalProjectID := os.Getenv("PROJECT_ID")
	originalGoogleProject := os.Getenv("GOOGLE_CLOUD_PROJECT")
	originalAWSRegion := os.Getenv("AWS_REGION")
	originalKubeconfig := os.Getenv("KUBECONFIG")
	originalK8SEndpoint := os.Getenv("K8S_ENDPOINT")
	originalK8SToken := os.Getenv("K8S_TOKEN")
	originalK8SCACert := os.Getenv("K8S_CA_CERT")
	defer func() {
		restoreEnv("PROJECT_ID", originalProjectID)
		restoreEnv("GOOGLE_CLOUD_PROJECT", originalGoogleProject)
		restoreEnv("AWS_REGION", originalAWSRegion)
		restoreEnv("KUBECONFIG", originalKubeconfig)
		restoreEnv("K8S_ENDPOINT", originalK8SEndpoint)
		restoreEnv("K8S_TOKEN", originalK8SToken)
		restoreEnv("K8S_CA_CERT", originalK8SCACert)
	}()

	// Clear all platform-related environment variables
	os.Unsetenv("PROJECT_ID")
	os.Unsetenv("GOOGLE_CLOUD_PROJECT")
	os.Unsetenv("AWS_REGION")
	os.Unsetenv("KUBECONFIG")
	os.Unsetenv("K8S_ENDPOINT")
	os.Unsetenv("K8S_TOKEN")
	os.Unsetenv("K8S_CA_CERT")

	// Test platform detection
	platform, err := DetectPlatform()

	if err == nil {
		t.Error("DetectPlatform() expected error when no platform env vars set, got nil")
	}

	if platform != Unknown {
		t.Errorf("DetectPlatform() = %v, want %v when error occurs", platform, Unknown)
	}

	// Verify error message is helpful
	expectedErrSubstring := "cannot detect platform"
	if err != nil && len(err.Error()) > 0 {
		errMsg := err.Error()
		if len(errMsg) < len(expectedErrSubstring) || errMsg[:len(expectedErrSubstring)] != expectedErrSubstring {
			t.Errorf("Error message should start with '%s', got '%s'", expectedErrSubstring, errMsg)
		}
	}
}

// TestPlatformString tests Platform.String() method
func TestPlatformString(t *testing.T) {
	tests := []struct {
		platform Platform
		want     string
	}{
		{GCP, "GCP"},
		{AWS, "AWS"},
		{Generic, "Generic"},
		{Unknown, "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.platform.String(); got != tt.want {
				t.Errorf("Platform.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

// restoreEnv is a helper to restore environment variables
func restoreEnv(key, value string) {
	if value == "" {
		os.Unsetenv(key)
	} else {
		os.Setenv(key, value)
	}
}
