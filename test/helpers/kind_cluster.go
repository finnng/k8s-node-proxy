package helpers

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// KindCluster manages a kind (Kubernetes IN Docker) cluster for testing
type KindCluster struct {
	Name       string
	ConfigPath string
	Kubeconfig string
}

// KindClusterConfig contains configuration for creating a kind cluster
type KindClusterConfig struct {
	Name        string
	Nodes       int    // Number of worker nodes
	ConfigPath  string // Optional: path to custom kind config
	WaitTimeout time.Duration
	Image       string // Optional: custom node image
}

// NewKindCluster creates a new kind cluster manager
func NewKindCluster(name string) *KindCluster {
	return &KindCluster{
		Name: name,
	}
}

// Create creates a new kind cluster
func (k *KindCluster) Create(ctx context.Context, config *KindClusterConfig) error {
	// Check if kind is installed
	if !k.isKindInstalled() {
		return fmt.Errorf("kind is not installed. Install with: go install sigs.k8s.io/kind@latest")
	}

	// Check if cluster already exists
	exists, err := k.Exists(ctx)
	if err != nil {
		return fmt.Errorf("failed to check if cluster exists: %w", err)
	}

	if exists {
		return fmt.Errorf("cluster %s already exists", k.Name)
	}

	// Build kind create command
	args := []string{"create", "cluster", "--name", k.Name}

	if config != nil {
		if config.ConfigPath != "" {
			args = append(args, "--config", config.ConfigPath)
		}
		if config.Image != "" {
			args = append(args, "--image", config.Image)
		}
		if config.WaitTimeout > 0 {
			args = append(args, "--wait", config.WaitTimeout.String())
		}
	}

	// Create cluster
	cmd := exec.CommandContext(ctx, "kind", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create kind cluster: %w", err)
	}

	// Get kubeconfig path
	k.Kubeconfig, err = k.GetKubeconfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	return nil
}

// Delete deletes the kind cluster
func (k *KindCluster) Delete(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "kind", "delete", "cluster", "--name", k.Name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to delete kind cluster: %w", err)
	}

	return nil
}

// Exists checks if the kind cluster exists
func (k *KindCluster) Exists(ctx context.Context) (bool, error) {
	cmd := exec.CommandContext(ctx, "kind", "get", "clusters")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("failed to list kind clusters: %w", err)
	}

	clusters := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, cluster := range clusters {
		if cluster == k.Name {
			return true, nil
		}
	}

	return false, nil
}

// GetKubeconfig returns the kubeconfig for the kind cluster
func (k *KindCluster) GetKubeconfig(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "kind", "get", "kubeconfig", "--name", k.Name)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	return string(output), nil
}

// GetKubeconfigPath writes the kubeconfig to a temp file and returns the path
func (k *KindCluster) GetKubeconfigPath(ctx context.Context) (string, error) {
	kubeconfig, err := k.GetKubeconfig(ctx)
	if err != nil {
		return "", err
	}

	// Create temp file
	tmpFile, err := os.CreateTemp("", fmt.Sprintf("kind-kubeconfig-%s-*.yaml", k.Name))
	if err != nil {
		return "", fmt.Errorf("failed to create temp kubeconfig file: %w", err)
	}
	defer tmpFile.Close()

	// Write kubeconfig
	if _, err := tmpFile.WriteString(kubeconfig); err != nil {
		return "", fmt.Errorf("failed to write kubeconfig: %w", err)
	}

	return tmpFile.Name(), nil
}

// LoadImage loads a Docker image into the kind cluster
func (k *KindCluster) LoadImage(ctx context.Context, imageName string) error {
	cmd := exec.CommandContext(ctx, "kind", "load", "docker-image", imageName, "--name", k.Name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to load image %s into kind cluster: %w", imageName, err)
	}

	return nil
}

// WaitForReady waits for the cluster to be ready
func (k *KindCluster) WaitForReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// Check if cluster is ready by trying to get nodes
		kubeconfigPath, err := k.GetKubeconfigPath(ctx)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		defer os.Remove(kubeconfigPath)

		cmd := exec.CommandContext(ctx, "kubectl", "get", "nodes", "--kubeconfig", kubeconfigPath)
		if err := cmd.Run(); err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		// Cluster is ready
		return nil
	}

	return fmt.Errorf("cluster did not become ready within %s", timeout)
}

// isKindInstalled checks if kind is installed
func (k *KindCluster) isKindInstalled() bool {
	cmd := exec.Command("kind", "version")
	return cmd.Run() == nil
}

// CreateWithConfig creates a kind cluster with a custom configuration file content
func (k *KindCluster) CreateWithConfig(ctx context.Context, configContent string, timeout time.Duration) error {
	// Write config to temp file
	tmpFile, err := os.CreateTemp("", fmt.Sprintf("kind-config-%s-*.yaml", k.Name))
	if err != nil {
		return fmt.Errorf("failed to create temp config file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if _, err := tmpFile.WriteString(configContent); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	config := &KindClusterConfig{
		Name:        k.Name,
		ConfigPath:  tmpFile.Name(),
		WaitTimeout: timeout,
	}

	return k.Create(ctx, config)
}

// GetDefaultKindConfig returns a default kind cluster configuration
// This creates a cluster with 1 control plane and 2 worker nodes
func GetDefaultKindConfig() string {
	return `kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
- role: worker
- role: worker
`
}

// GetSingleNodeKindConfig returns a single-node kind cluster configuration
func GetSingleNodeKindConfig() string {
	return `kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
`
}

// GetMultiNodeKindConfig returns a multi-node kind cluster configuration
// with specified number of worker nodes
func GetMultiNodeKindConfig(workerNodes int) string {
	config := `kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
`
	for i := 0; i < workerNodes; i++ {
		config += "- role: worker\n"
	}

	return config
}

// CleanupAllKindClusters deletes all kind clusters (useful for test cleanup)
func CleanupAllKindClusters(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "kind", "get", "clusters")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list kind clusters: %w", err)
	}

	clusters := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, cluster := range clusters {
		if cluster == "" {
			continue
		}
		k := NewKindCluster(cluster)
		if err := k.Delete(ctx); err != nil {
			return err
		}
	}

	return nil
}
