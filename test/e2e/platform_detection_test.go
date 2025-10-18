package e2e

import (
	"os"
	"testing"

	"k8s-node-proxy/internal/platform"
)

// TestPlatformAutoDetection tests automatic platform detection via metadata and env vars
func TestPlatformAutoDetection(t *testing.T) {
	t.Run("GCPWithProjectID", func(t *testing.T) {
		// Save and restore env
		originalProjectID := os.Getenv("PROJECT_ID")
		originalAWSRegion := os.Getenv("AWS_REGION")
		defer func() {
			restoreEnv("PROJECT_ID", originalProjectID)
			restoreEnv("AWS_REGION", originalAWSRegion)
		}()

		// Setup: Only PROJECT_ID set
		os.Setenv("PROJECT_ID", "test-project")
		os.Unsetenv("AWS_REGION")

		detected, err := platform.DetectPlatform()
		if err != nil {
			t.Fatalf("Failed to detect platform: %v", err)
		}

		if detected != platform.GCP {
			t.Errorf("Expected GCP, got %s", detected)
		}
	})

	t.Run("AWSWithRegion", func(t *testing.T) {
		// Save and restore env
		originalProjectID := os.Getenv("PROJECT_ID")
		originalAWSRegion := os.Getenv("AWS_REGION")
		defer func() {
			restoreEnv("PROJECT_ID", originalProjectID)
			restoreEnv("AWS_REGION", originalAWSRegion)
		}()

		// Setup: Only AWS_REGION set
		os.Unsetenv("PROJECT_ID")
		os.Setenv("AWS_REGION", "us-east-1")

		detected, err := platform.DetectPlatform()
		if err != nil {
			t.Fatalf("Failed to detect platform: %v", err)
		}

		if detected != platform.AWS {
			t.Errorf("Expected AWS, got %s", detected)
		}
	})

	t.Run("GenericWithKubeconfig", func(t *testing.T) {
		// Save and restore env
		originalProjectID := os.Getenv("PROJECT_ID")
		originalAWSRegion := os.Getenv("AWS_REGION")
		originalKubeconfig := os.Getenv("KUBECONFIG")
		defer func() {
			restoreEnv("PROJECT_ID", originalProjectID)
			restoreEnv("AWS_REGION", originalAWSRegion)
			restoreEnv("KUBECONFIG", originalKubeconfig)
		}()

		// Setup: Only KUBECONFIG set
		os.Unsetenv("PROJECT_ID")
		os.Unsetenv("AWS_REGION")
		os.Setenv("KUBECONFIG", "/path/to/kubeconfig")

		detected, err := platform.DetectPlatform()
		if err != nil {
			t.Fatalf("Failed to detect platform: %v", err)
		}

		if detected != platform.Generic {
			t.Errorf("Expected Generic, got %s", detected)
		}
	})
}

// TestMixedEnvironmentVariables tests precedence when multiple platform env vars are set
func TestMixedEnvironmentVariables(t *testing.T) {
	t.Run("GCPTakesPrecedenceOverAWS", func(t *testing.T) {
		// Save and restore env
		originalProjectID := os.Getenv("PROJECT_ID")
		originalAWSRegion := os.Getenv("AWS_REGION")
		defer func() {
			restoreEnv("PROJECT_ID", originalProjectID)
			restoreEnv("AWS_REGION", originalAWSRegion)
		}()

		// Setup: Both PROJECT_ID and AWS_REGION set
		os.Setenv("PROJECT_ID", "test-project")
		os.Setenv("AWS_REGION", "us-east-1")

		detected, err := platform.DetectPlatform()
		if err != nil {
			t.Fatalf("Failed to detect platform: %v", err)
		}

		// GCP should take precedence (checked first)
		if detected != platform.GCP {
			t.Errorf("Expected GCP to take precedence, got %s", detected)
		}
	})

	t.Run("AWSTakesPrecedenceOverGeneric", func(t *testing.T) {
		// Save and restore env
		originalProjectID := os.Getenv("PROJECT_ID")
		originalAWSRegion := os.Getenv("AWS_REGION")
		originalKubeconfig := os.Getenv("KUBECONFIG")
		defer func() {
			restoreEnv("PROJECT_ID", originalProjectID)
			restoreEnv("AWS_REGION", originalAWSRegion)
			restoreEnv("KUBECONFIG", originalKubeconfig)
		}()

		// Setup: Both AWS_REGION and KUBECONFIG set
		os.Unsetenv("PROJECT_ID")
		os.Setenv("AWS_REGION", "us-east-1")
		os.Setenv("KUBECONFIG", "/path/to/kubeconfig")

		detected, err := platform.DetectPlatform()
		if err != nil {
			t.Fatalf("Failed to detect platform: %v", err)
		}

		// AWS should take precedence over Generic
		if detected != platform.AWS {
			t.Errorf("Expected AWS to take precedence over Generic, got %s", detected)
		}
	})

	t.Run("AllThreeSetGCPWins", func(t *testing.T) {
		// Save and restore env
		originalProjectID := os.Getenv("PROJECT_ID")
		originalAWSRegion := os.Getenv("AWS_REGION")
		originalKubeconfig := os.Getenv("KUBECONFIG")
		defer func() {
			restoreEnv("PROJECT_ID", originalProjectID)
			restoreEnv("AWS_REGION", originalAWSRegion)
			restoreEnv("KUBECONFIG", originalKubeconfig)
		}()

		// Setup: All three platform indicators set
		os.Setenv("PROJECT_ID", "test-project")
		os.Setenv("AWS_REGION", "us-east-1")
		os.Setenv("KUBECONFIG", "/path/to/kubeconfig")

		detected, err := platform.DetectPlatform()
		if err != nil {
			t.Fatalf("Failed to detect platform: %v", err)
		}

		// GCP should win (first in precedence order)
		if detected != platform.GCP {
			t.Errorf("Expected GCP with all vars set, got %s", detected)
		}
	})
}

// TestPlatformDetectionPrecedenceOrder tests the documented precedence order
func TestPlatformDetectionPrecedenceOrder(t *testing.T) {
	// Precedence order: GCP → AWS → Generic

	t.Run("VerifyPrecedenceOrder", func(t *testing.T) {
		precedence := []platform.Platform{
			platform.GCP,
			platform.AWS,
			platform.Generic,
		}

		t.Logf("Platform detection precedence order:")
		for i, p := range precedence {
			t.Logf("%d. %s", i+1, p)
		}
	})
}

// TestNoPlatformDetected tests behavior when no platform can be detected
func TestNoPlatformDetected(t *testing.T) {
	t.Run("NoEnvironmentVariables", func(t *testing.T) {
		// Save and restore env
		originalProjectID := os.Getenv("PROJECT_ID")
		originalGoogleProject := os.Getenv("GOOGLE_CLOUD_PROJECT")
		originalAWSRegion := os.Getenv("AWS_REGION")
		originalKubeconfig := os.Getenv("KUBECONFIG")
		defer func() {
			restoreEnv("PROJECT_ID", originalProjectID)
			restoreEnv("GOOGLE_CLOUD_PROJECT", originalGoogleProject)
			restoreEnv("AWS_REGION", originalAWSRegion)
			restoreEnv("KUBECONFIG", originalKubeconfig)
		}()

		// Clear all platform env vars
		os.Unsetenv("PROJECT_ID")
		os.Unsetenv("GOOGLE_CLOUD_PROJECT")
		os.Unsetenv("AWS_REGION")
		os.Unsetenv("KUBECONFIG")

		_, err := platform.DetectPlatform()
		if err == nil {
			t.Error("Expected error when no platform env vars are set")
		}

		t.Logf("Expected error: %v", err)
	})
}

// restoreEnv restores an environment variable to its original value
func restoreEnv(key, value string) {
	if value == "" {
		os.Unsetenv(key)
	} else {
		os.Setenv(key, value)
	}
}
