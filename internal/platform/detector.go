package platform

import (
	"fmt"
	"os"
)

// Platform represents the cloud platform where the proxy is running
type Platform int

const (
	// Unknown represents an unknown or unsupported platform
	Unknown Platform = iota
	// GCP represents Google Cloud Platform (GKE)
	GCP
	// AWS represents Amazon Web Services (EKS)
	AWS
	// Generic represents any Kubernetes cluster using kubeconfig
	Generic
)

// String returns the string representation of the Platform
func (p Platform) String() string {
	switch p {
	case GCP:
		return "GCP"
	case AWS:
		return "AWS"
	case Generic:
		return "Generic"
	default:
		return "Unknown"
	}
}

// DetectPlatform determines the cloud platform based on environment variables
// It checks in the following order:
// 1. PROJECT_ID or GOOGLE_CLOUD_PROJECT → GCP
// 2. AWS_REGION → AWS
// 3. KUBECONFIG or K8S_* env vars → Generic
// 4. Neither → Error
//
// This is a simple, happy-path implementation for Phase 1.
// Metadata service detection will be added in Phase 4.
func DetectPlatform() (Platform, error) {
	// Check for GCP first (PROJECT_ID takes precedence)
	projectID := os.Getenv("PROJECT_ID")
	if projectID != "" {
		return GCP, nil
	}

	// Check GOOGLE_CLOUD_PROJECT as alternative
	googleProject := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if googleProject != "" {
		return GCP, nil
	}

	// Check for AWS
	awsRegion := os.Getenv("AWS_REGION")
	if awsRegion != "" {
		return AWS, nil
	}

	// Check for Generic Kubernetes (kubeconfig-based)
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig != "" {
		return Generic, nil
	}

	// Check for alternative K8S_* environment variables
	k8sEndpoint := os.Getenv("K8S_ENDPOINT")
	k8sToken := os.Getenv("K8S_TOKEN")
	k8sCACert := os.Getenv("K8S_CA_CERT")
	if k8sEndpoint != "" && k8sToken != "" && k8sCACert != "" {
		return Generic, nil
	}

	// Check for in-cluster Kubernetes configuration (when running as a pod)
	// Kubernetes automatically mounts service account tokens at this path
	serviceAccountTokenPath := "/var/run/secrets/kubernetes.io/serviceaccount/token"
	if _, err := os.Stat(serviceAccountTokenPath); err == nil {
		return Generic, nil
	}

	// No platform detected
	return Unknown, fmt.Errorf("cannot detect platform: neither GCP (PROJECT_ID/GOOGLE_CLOUD_PROJECT), AWS (AWS_REGION), nor Generic Kubernetes (KUBECONFIG or K8S_* env vars) environment variables are set")
}
