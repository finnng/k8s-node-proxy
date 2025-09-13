package discovery

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"
	"sync"
	"time"

	"google.golang.org/api/container/v1"
	"google.golang.org/api/option"
	"golang.org/x/oauth2/google"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type NodeDiscovery struct {
	projectID    string
	containerSvc *container.Service
	k8sClientset *kubernetes.Clientset
	cachedIP     string
	cacheTime    time.Time
	cacheTTL     time.Duration
	mutex        sync.RWMutex
}

func New(projectID string) (*NodeDiscovery, error) {
	ctx := context.Background()

	containerSvc, err := container.NewService(ctx, option.WithScopes(container.CloudPlatformScope))
	if err != nil {
		return nil, fmt.Errorf("failed to create container service: %w", err)
	}

	// Build Kubernetes config
	config, _, err := buildK8sConfig(ctx, containerSvc, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to build K8s config: %w", err)
	}

	k8sClientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create K8s clientset: %w", err)
	}

	return &NodeDiscovery{
		projectID:    projectID,
		containerSvc: containerSvc,
		k8sClientset: k8sClientset,
		cacheTTL:     5 * time.Minute,
	}, nil
}

func (d *NodeDiscovery) GetCurrentNodeIP(ctx context.Context) (string, error) {
	d.mutex.RLock()
	if d.cachedIP != "" && time.Since(d.cacheTime) < d.cacheTTL {
		ip := d.cachedIP
		d.mutex.RUnlock()
		return ip, nil
	}
	d.mutex.RUnlock()

	d.mutex.Lock()
	defer d.mutex.Unlock()

	if d.cachedIP != "" && time.Since(d.cacheTime) < d.cacheTTL {
		return d.cachedIP, nil
	}

	ip, err := d.discoverNodeIP(ctx)
	if err != nil {
		return "", err
	}

	d.cachedIP = ip
	d.cacheTime = time.Now()
	return ip, nil
}

func (d *NodeDiscovery) discoverNodeIP(ctx context.Context) (string, error) {
	// Use Kubernetes API instead of cloud provider API for platform independence
	nodes, err := d.k8sClientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to list nodes: %w", err)
	}

	if len(nodes.Items) == 0 {
		return "", fmt.Errorf("no nodes found in cluster")
	}

	// Select oldest node (by CreationTimestamp)
	oldestNode := findOldestNode(nodes.Items)

	// Get Internal IP (same as original GCE NetworkIP behavior)
	nodeIP := getNodeInternalIP(oldestNode)
	if nodeIP == "" {
		return "", fmt.Errorf("no usable IP found for node %s", oldestNode.Name)
	}

	return nodeIP, nil
}

// getNodeInternalIP extracts the Internal IP (matching original GCE NetworkIP behavior)
func getNodeInternalIP(node corev1.Node) string {
	// Get Internal IP (equivalent to GCE NetworkIP)
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			return addr.Address
		}
	}

	return ""
}

// findOldestNode returns the node with the earliest creation timestamp
func findOldestNode(nodes []corev1.Node) corev1.Node {
	if len(nodes) == 0 {
		return corev1.Node{}
	}

	// Sort nodes by creation time (oldest first)
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].CreationTimestamp.Before(&nodes[j].CreationTimestamp)
	})

	return nodes[0]
}

// buildK8sConfig creates Kubernetes client configuration
func buildK8sConfig(ctx context.Context, containerSvc *container.Service, projectID string) (*rest.Config, interface{}, error) {
	// Get the first cluster in the project
	clusters, err := containerSvc.Projects.Locations.Clusters.List(
		fmt.Sprintf("projects/%s/locations/-", projectID)).Context(ctx).Do()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list clusters: %w", err)
	}

	if len(clusters.Clusters) == 0 {
		return nil, nil, fmt.Errorf("no clusters found in project %s", projectID)
	}

	cluster := clusters.Clusters[0]

	// Decode cluster CA certificate
	caCert, err := base64.StdEncoding.DecodeString(cluster.MasterAuth.ClusterCaCertificate)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode cluster CA certificate: %w", err)
	}

	// Get Google default token source (uses ADC)
	tokenSource, err := google.DefaultTokenSource(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get default token source: %w", err)
	}

	// Get a token to use for authentication
	token, err := tokenSource.Token()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get token: %w", err)
	}

	// Build Kubernetes config
	config := &rest.Config{
		Host: "https://" + cluster.Endpoint,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: caCert,
		},
		BearerToken: token.AccessToken,
	}

	return config, nil, nil
}
