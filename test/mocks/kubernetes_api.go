package mocks

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MockK8sNode represents a mock Kubernetes node for testing
type MockK8sNode struct {
	Name         string
	InternalIP   string
	IsHealthy    bool
	CreationTime time.Time
}

// NewMockKubernetesAPI creates a test HTTP server that mocks Kubernetes API
func NewMockKubernetesAPI(nodes []MockK8sNode, services []corev1.Service) *httptest.Server {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle node list requests
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/api/v1/nodes") {
			handleNodesList(w, r, nodes)
			return
		}

		// Handle service list requests
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/api/v1/namespaces/") && strings.Contains(r.URL.Path, "/services") {
			handleServicesList(w, r, services)
			return
		}

		// Default response for unmatched endpoints
		http.NotFound(w, r)
	})

	return httptest.NewServer(handler)
}

// handleNodesList returns a list of nodes
func handleNodesList(w http.ResponseWriter, r *http.Request, mockNodes []MockK8sNode) {
	w.Header().Set("Content-Type", "application/json")

	nodes := make([]corev1.Node, len(mockNodes))
	for i, mockNode := range mockNodes {
		nodes[i] = createK8sNode(mockNode)
	}

	nodeList := &corev1.NodeList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "NodeList",
			APIVersion: "v1",
		},
		Items: nodes,
	}

	json.NewEncoder(w).Encode(nodeList)
}

// handleServicesList returns a list of services
func handleServicesList(w http.ResponseWriter, r *http.Request, mockServices []corev1.Service) {
	w.Header().Set("Content-Type", "application/json")

	serviceList := &corev1.ServiceList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceList",
			APIVersion: "v1",
		},
		Items: mockServices,
	}

	json.NewEncoder(w).Encode(serviceList)
}

// createK8sNode creates a Kubernetes Node object from mock data
func createK8sNode(mockNode MockK8sNode) corev1.Node {
	nodeStatus := corev1.ConditionTrue
	if !mockNode.IsHealthy {
		nodeStatus = corev1.ConditionFalse
	}

	return corev1.Node{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Node",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              mockNode.Name,
			CreationTimestamp: metav1.NewTime(mockNode.CreationTime),
		},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{
				{
					Type:    corev1.NodeInternalIP,
					Address: mockNode.InternalIP,
				},
			},
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: nodeStatus,
				},
			},
		},
	}
}

// NewDefaultMockNodes creates a set of mock nodes for testing
func NewDefaultMockNodes() []MockK8sNode {
	now := time.Now()

	return []MockK8sNode{
		{
			Name:         "gke-test-node-1",
			InternalIP:   "10.0.1.1",
			IsHealthy:    true,
			CreationTime: now.Add(-48 * time.Hour), // Oldest node
		},
		{
			Name:         "gke-test-node-2",
			InternalIP:   "10.0.1.2",
			IsHealthy:    true,
			CreationTime: now.Add(-24 * time.Hour),
		},
		{
			Name:         "gke-test-node-3",
			InternalIP:   "10.0.1.3",
			IsHealthy:    true,
			CreationTime: now.Add(-12 * time.Hour), // Newest node
		},
	}
}

// NewMockNodePortService creates a mock NodePort service
func NewMockNodePortService(name, namespace string, nodePort int32) corev1.Service {
	return corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
			Ports: []corev1.ServicePort{
				{
					Name:     "http",
					Protocol: corev1.ProtocolTCP,
					Port:     80,
					NodePort: nodePort,
				},
			},
		},
	}
}
