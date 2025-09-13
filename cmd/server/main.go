package main

import (
	"log"
	"os"

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

	log.Printf("Starting k8s-node-proxy for project: %s", projectID)

	srv, err := server.New(projectID)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	if err := srv.Run(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
