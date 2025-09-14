package main

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
	"k8s-node-proxy/internal/server"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Printf("No .env file found or error loading it: %v", err)
	}

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

	log.Printf("Starting k8s-node-proxy for project: %s, service port: %d", projectID, proxyServicePort)

	srv, err := server.New(projectID, proxyServicePort)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	if err := srv.Run(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
