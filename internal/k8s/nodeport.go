package k8s

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/container/v1"
	"google.golang.org/api/option"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type ServiceInfo struct {
	Name       string
	Namespace  string
	NodePort   int32
	TargetPort int32
	Protocol   string
}

type ClusterInfo struct {
	Name     string
	Location string
	Endpoint string
}

type NodePortDiscovery struct {
	projectID    string
	containerSvc *container.Service
	k8sClientset *kubernetes.Clientset
	clusterInfo  *ClusterInfo
}

func NewNodePortDiscovery(projectID string) (*NodePortDiscovery, error) {
	slog.Info("Initializing NodePort discovery", "project", projectID)

	ctx := context.Background()
	containerSvc, err := container.NewService(ctx, option.WithScopes(container.CloudPlatformScope))
	if err != nil {
		return nil, fmt.Errorf("failed to create container service: %w", err)
	}

	config, clusterInfo, err := buildK8sConfig(ctx, containerSvc, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to build K8s config: %w", err)
	}

	k8sClientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create K8s clientset: %w", err)
	}

	slog.Info("NodePort discovery initialized successfully")
	return &NodePortDiscovery{
		projectID:    projectID,
		containerSvc: containerSvc,
		k8sClientset: k8sClientset,
		clusterInfo:  clusterInfo,
	}, nil
}

func buildK8sConfig(ctx context.Context, containerSvc *container.Service, projectID string) (*rest.Config, *ClusterInfo, error) {
	slog.Info("Building Kubernetes client configuration")

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
	slog.Info("Using cluster for K8s API access", "cluster", cluster.Name, "location", cluster.Location)

	// Create cluster info
	clusterInfo := &ClusterInfo{
		Name:     cluster.Name,
		Location: cluster.Location,
		Endpoint: cluster.Endpoint,
	}

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

	slog.Info("Kubernetes configuration built successfully", "endpoint", cluster.Endpoint)
	return config, clusterInfo, nil
}

func (d *NodePortDiscovery) DiscoverNodePorts(ctx context.Context) ([]int, error) {
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

func (d *NodePortDiscovery) DiscoverServices(ctx context.Context) ([]ServiceInfo, error) {
	slog.Info("Obtaining available node ports")

	services, err := d.k8sClientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
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

	slog.Info("NodePort discovery completed", "total_services", len(serviceInfos))
	return serviceInfos, nil
}

func (d *NodePortDiscovery) GetClusterInfo() *ClusterInfo {
	return d.clusterInfo
}
