package services

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestNewGenericNodePortDiscovery_Kubeconfig tests initialization with KUBECONFIG (T030)
func TestNewGenericNodePortDiscovery_Kubeconfig(t *testing.T) {
	// Save original env vars
	originalKubeconfig := os.Getenv("KUBECONFIG")
	originalK8SEndpoint := os.Getenv("K8S_ENDPOINT")
	originalK8SToken := os.Getenv("K8S_TOKEN")
	originalK8SCACert := os.Getenv("K8S_CA_CERT")
	defer func() {
		restoreEnv("KUBECONFIG", originalKubeconfig)
		restoreEnv("K8S_ENDPOINT", originalK8SEndpoint)
		restoreEnv("K8S_TOKEN", originalK8SToken)
		restoreEnv("K8S_CA_CERT", originalK8SCACert)
	}()

	// Clear all env vars first
	os.Unsetenv("KUBECONFIG")
	os.Unsetenv("K8S_ENDPOINT")
	os.Unsetenv("K8S_TOKEN")
	os.Unsetenv("K8S_CA_CERT")

	// Test with KUBECONFIG set
	os.Setenv("KUBECONFIG", "/path/to/kubeconfig")

	// This will fail because the kubeconfig file doesn't exist, but we can test the path
	_, err := NewGenericNodePortDiscovery()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to build config from kubeconfig")
}

// TestNewGenericNodePortDiscovery_EnvVars tests initialization with environment variables (T031)
func TestNewGenericNodePortDiscovery_EnvVars(t *testing.T) {
	// Save original env vars
	originalKubeconfig := os.Getenv("KUBECONFIG")
	originalK8SEndpoint := os.Getenv("K8S_ENDPOINT")
	originalK8SToken := os.Getenv("K8S_TOKEN")
	originalK8SCACert := os.Getenv("K8S_CA_CERT")
	defer func() {
		restoreEnv("KUBECONFIG", originalKubeconfig)
		restoreEnv("K8S_ENDPOINT", originalK8SEndpoint)
		restoreEnv("K8S_TOKEN", originalK8SToken)
		restoreEnv("K8S_CA_CERT", originalK8SCACert)
	}()

	// Clear all env vars first
	os.Unsetenv("KUBECONFIG")
	os.Unsetenv("K8S_ENDPOINT")
	os.Unsetenv("K8S_TOKEN")
	os.Unsetenv("K8S_CA_CERT")

	// Test with incomplete env vars - should fall back to in-cluster config
	os.Setenv("K8S_ENDPOINT", "https://k8s.example.com:6443")
	os.Setenv("K8S_TOKEN", "eyJhbGciOiJSUzI1NiIsImtpZCI6...")

	_, err := NewGenericNodePortDiscovery()
	assert.Error(t, err)
	// Should get in-cluster config error, not env var validation error
	assert.Contains(t, err.Error(), "in-cluster")

	// Test with complete env vars
	os.Setenv("K8S_CA_CERT", "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t...")

	// This will fail because the endpoint is not reachable, but we can test the initialization path
	_, err = NewGenericNodePortDiscovery()
	assert.Error(t, err)
	// Should not be the env var validation error
	assert.NotContains(t, err.Error(), "environment variables must be set")
}

// TestNewGenericNodePortDiscovery_NoConfig tests fallback to in-cluster when no config is provided (T032)
func TestNewGenericNodePortDiscovery_NoConfig(t *testing.T) {
	// Save original env vars
	originalKubeconfig := os.Getenv("KUBECONFIG")
	originalK8SEndpoint := os.Getenv("K8S_ENDPOINT")
	originalK8SToken := os.Getenv("K8S_TOKEN")
	originalK8SCACert := os.Getenv("K8S_CA_CERT")
	defer func() {
		restoreEnv("KUBECONFIG", originalKubeconfig)
		restoreEnv("K8S_ENDPOINT", originalK8SEndpoint)
		restoreEnv("K8S_TOKEN", originalK8SToken)
		restoreEnv("K8S_CA_CERT", originalK8SCACert)
	}()

	// Clear all env vars
	os.Unsetenv("KUBECONFIG")
	os.Unsetenv("K8S_ENDPOINT")
	os.Unsetenv("K8S_TOKEN")
	os.Unsetenv("K8S_CA_CERT")

	// Should fall back to in-cluster config, which will fail in test environment
	_, err := NewGenericNodePortDiscovery()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "in-cluster")
}

// restoreEnv is a helper to restore environment variables
func restoreEnv(key, value string) {
	if value == "" {
		os.Unsetenv(key)
	} else {
		os.Setenv(key, value)
	}
}
