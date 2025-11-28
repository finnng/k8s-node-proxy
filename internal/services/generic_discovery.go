package services

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// GenericNodePortDiscovery implements service discovery for any Kubernetes cluster using kubeconfig
type GenericNodePortDiscovery struct {
	kubeconfig   string
	k8sEndpoint  string
	k8sToken     string
	k8sCACert    string
	k8sClientset *kubernetes.Clientset
	clusterInfo  *ClusterInfo
}

// NewGenericNodePortDiscovery creates a new generic Kubernetes service discovery instance
func NewGenericNodePortDiscovery() (*GenericNodePortDiscovery, error) {
	slog.Info("Initializing Generic Kubernetes NodePort discovery")

	// Try kubeconfig first
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig != "" {
		slog.Info("Using kubeconfig for authentication", "path", kubeconfig)
		return newGenericDiscoveryFromKubeconfig(kubeconfig)
	}

	// Try individual environment variables
	k8sEndpoint := os.Getenv("K8S_ENDPOINT")
	k8sToken := os.Getenv("K8S_TOKEN")
	k8sCACert := os.Getenv("K8S_CA_CERT")

	if k8sEndpoint != "" && k8sToken != "" && k8sCACert != "" {
		slog.Info("Using environment variables for authentication")
		return newGenericDiscoveryFromEnv(k8sEndpoint, k8sToken, k8sCACert)
	}

	// Try in-cluster configuration (when running as a pod)
	slog.Info("Attempting in-cluster configuration")
	return newGenericDiscoveryFromInCluster()
}

// newGenericDiscoveryFromKubeconfig creates discovery using kubeconfig file
func newGenericDiscoveryFromKubeconfig(kubeconfigPath string) (*GenericNodePortDiscovery, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build config from kubeconfig: %w", err)
	}

	k8sClientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create K8s clientset: %w", err)
	}

	// Extract cluster info from config
	clusterInfo := &ClusterInfo{
		Name:     "generic-cluster", // Could be extracted from kubeconfig if needed
		Location: "generic",
		Endpoint: config.Host,
	}

	slog.Info("Generic Kubernetes discovery initialized with kubeconfig", "endpoint", config.Host)
	return &GenericNodePortDiscovery{
		kubeconfig:   kubeconfigPath,
		k8sClientset: k8sClientset,
		clusterInfo:  clusterInfo,
	}, nil
}

// newGenericDiscoveryFromEnv creates discovery using environment variables
func newGenericDiscoveryFromEnv(endpoint, token, caCert string) (*GenericNodePortDiscovery, error) {
	// Decode base64 CA certificate if needed
	caCertBytes := []byte(caCert)
	if len(caCert) > 0 && caCert[:10] != "-----BEGIN" {
		// Assume it's base64 encoded
		decoded, err := base64.StdEncoding.DecodeString(caCert)
		if err != nil {
			return nil, fmt.Errorf("failed to decode CA certificate: %w", err)
		}
		caCertBytes = decoded
	}

	config := &rest.Config{
		Host:        endpoint,
		BearerToken: token,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: caCertBytes,
		},
	}

	k8sClientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create K8s clientset: %w", err)
	}

	clusterInfo := &ClusterInfo{
		Name:     "generic-cluster",
		Location: "generic",
		Endpoint: endpoint,
	}

	slog.Info("Generic Kubernetes discovery initialized with env vars", "endpoint", endpoint)
	return &GenericNodePortDiscovery{
		k8sEndpoint:  endpoint,
		k8sToken:     token,
		k8sCACert:    caCert,
		k8sClientset: k8sClientset,
		clusterInfo:  clusterInfo,
	}, nil
}

// newGenericDiscoveryFromInCluster creates discovery using in-cluster configuration
func newGenericDiscoveryFromInCluster() (*GenericNodePortDiscovery, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
	}

	k8sClientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create K8s clientset from in-cluster config: %w", err)
	}

	clusterInfo := &ClusterInfo{
		Name:     "in-cluster",
		Location: "in-cluster",
		Endpoint: config.Host,
	}

	slog.Info("Generic Kubernetes discovery initialized with in-cluster config", "endpoint", config.Host)
	return &GenericNodePortDiscovery{
		k8sClientset: k8sClientset,
		clusterInfo:  clusterInfo,
	}, nil
}

// DiscoverNodePorts discovers available NodePort services and returns their ports
func (d *GenericNodePortDiscovery) DiscoverNodePorts(ctx context.Context) ([]int, error) {
	services, err := d.DiscoverServices(ctx)
	if err != nil {
		return nil, err
	}

	var ports []int
	for _, service := range services {
		ports = append(ports, int(service.NodePort))
	}

	return ports, nil
}

// DiscoverServices discovers NodePort services in the cluster
func (d *GenericNodePortDiscovery) DiscoverServices(ctx context.Context) ([]ServiceInfo, error) {
	slog.Info("Discovering Generic Kubernetes NodePort services")

	// Get namespace from environment variable - required
	namespace := os.Getenv("NAMESPACE")
	if namespace == "" {
		return nil, fmt.Errorf("NAMESPACE environment variable is required")
	}

	slog.Info("Discovering services in namespace", "namespace", namespace)

	services, err := d.k8sClientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	var serviceInfos []ServiceInfo
	for _, service := range services.Items {
		if service.Spec.Type == corev1.ServiceTypeNodePort {
			for _, port := range service.Spec.Ports {
				if port.NodePort != 0 {
					serviceInfo := ServiceInfo{
						Name:       service.Name,
						Namespace:  service.Namespace,
						NodePort:   port.NodePort,
						TargetPort: port.TargetPort.IntVal,
						Protocol:   string(port.Protocol),
					}
					serviceInfos = append(serviceInfos, serviceInfo)
					slog.Info("Found NodePort service",
						"service", service.Name,
						"namespace", service.Namespace,
						"nodePort", port.NodePort,
						"targetPort", port.TargetPort.IntVal)
				}
			}
		}
	}

	slog.Info("Generic Kubernetes NodePort discovery completed", "total_services", len(serviceInfos))
	return serviceInfos, nil
}

// GetClientset returns the Kubernetes clientset used by this discovery
func (d *GenericNodePortDiscovery) GetClientset() *kubernetes.Clientset {
	return d.k8sClientset
}

// GetClusterInfo returns information about the generic Kubernetes cluster
func (d *GenericNodePortDiscovery) GetClusterInfo() *ClusterInfo {
	return d.clusterInfo
}
