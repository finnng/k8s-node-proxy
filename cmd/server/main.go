package main

import (
	"log"
	"os"
	"strconv"

	"k8s-node-proxy/internal/platform"
	"k8s-node-proxy/internal/server"
)

func main() {
	// Detect cloud platform (Phase 1: environment variable-based detection)
	detectedPlatform, err := platform.DetectPlatform()
	if err != nil {
		log.Fatalf("Platform detection failed: %v", err)
	}

	log.Printf("Detected platform: %s", detectedPlatform)

	// Route to appropriate platform-specific logic
	switch detectedPlatform {
	case platform.GCP:
		runGKEMode()
	case platform.AWS:
		runEKSMode()
	case platform.Generic:
		runGenericMode()
	default:
		log.Fatalf("Unsupported platform: %s", detectedPlatform)
	}
}

// runGKEMode runs the proxy in GKE mode (existing functionality, unchanged)
func runGKEMode() {
	projectID := os.Getenv("PROJECT_ID")
	if projectID == "" {
		projectID = os.Getenv("GOOGLE_CLOUD_PROJECT")
	}
	if projectID == "" {
		log.Fatal("PROJECT_ID or GOOGLE_CLOUD_PROJECT environment variable must be set")
	}

	// Get proxy service port from environment, default to 80
	proxyServicePort := 80
	if portStr := os.Getenv("PROXY_SERVICE_PORT"); portStr != "" {
		if port, err := strconv.Atoi(portStr); err != nil {
			log.Fatalf("Invalid PROXY_SERVICE_PORT value '%s': %v", portStr, err)
		} else {
			proxyServicePort = port
		}
	}

	log.Printf("Starting k8s-node-proxy for GKE project: %s, service port: %d", projectID, proxyServicePort)

	srv, err := server.New(projectID, proxyServicePort)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	if err := srv.Run(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

// runGenericMode runs the proxy in Generic Kubernetes mode
func runGenericMode() {
	log.Printf("Generic Kubernetes platform detected!")

	// Get proxy service port from environment, default to 80
	proxyServicePort := 80
	if portStr := os.Getenv("PROXY_SERVICE_PORT"); portStr != "" {
		if port, err := strconv.Atoi(portStr); err != nil {
			log.Fatalf("Invalid PROXY_SERVICE_PORT value '%s': %v", portStr, err)
		} else {
			proxyServicePort = port
		}
	}

	log.Printf("Starting k8s-node-proxy for Generic Kubernetes, service port: %d", proxyServicePort)

	srv, err := NewGenericServer(proxyServicePort)
	if err != nil {
		log.Fatalf("Failed to create generic server: %v", err)
	}

	if err := srv.Run(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

// runEKSMode runs the proxy in EKS mode
func runEKSMode() {
	log.Printf("AWS EKS platform detected!")

	// Get required AWS environment variables
	awsRegion := os.Getenv("AWS_REGION")
	if awsRegion == "" {
		log.Fatal("AWS_REGION environment variable must be set for EKS mode")
	}

	clusterName := os.Getenv("CLUSTER_NAME")
	if clusterName == "" {
		log.Fatal("CLUSTER_NAME environment variable must be set for EKS mode")
	}

	// Get proxy service port from environment, default to 80
	proxyServicePort := 80
	if portStr := os.Getenv("PROXY_SERVICE_PORT"); portStr != "" {
		if port, err := strconv.Atoi(portStr); err != nil {
			log.Fatalf("Invalid PROXY_SERVICE_PORT value '%s': %v", portStr, err)
		} else {
			proxyServicePort = port
		}
	}

	log.Printf("Starting k8s-node-proxy for EKS cluster: %s in region: %s, service port: %d", clusterName, awsRegion, proxyServicePort)

	srv, err := NewEKSServer(awsRegion, clusterName, proxyServicePort)
	if err != nil {
		log.Fatalf("Failed to create EKS server: %v", err)
	}

	if err := srv.Run(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
