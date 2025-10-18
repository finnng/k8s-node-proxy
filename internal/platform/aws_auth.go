package platform

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"sigs.k8s.io/aws-iam-authenticator/pkg/token"
)

// GenerateK8sToken generates an AWS IAM authenticator token for Kubernetes API authentication
func GenerateK8sToken() (string, error) {
	// Load AWS configuration
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return "", fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create STS client for token generation
	stsClient := sts.NewFromConfig(cfg)

	// Generate token using AWS IAM authenticator
	gen, err := token.NewGenerator(false, false)
	if err != nil {
		return "", fmt.Errorf("failed to create token generator: %w", err)
	}

	// Get the token
	tok, err := gen.GetWithSTS("https://sts.amazonaws.com", stsClient)
	if err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}

	return tok.Token, nil
}
