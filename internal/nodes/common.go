package nodes

import (
	corev1 "k8s.io/api/core/v1"
)

// getNodeStatus determines the health status from node conditions
// This function is shared across all platform implementations (GKE, Generic, EKS)
func getNodeStatus(node corev1.Node) NodeStatus {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			if condition.Status == corev1.ConditionTrue {
				return NodeHealthy
			}
			return NodeUnhealthy
		}
	}
	return NodeUnknown
}

// getNodeInternalIP extracts the Internal IP (matching original GCE NetworkIP behavior)
// This function is shared across all platform implementations (GKE, Generic, EKS)
func getNodeInternalIP(node corev1.Node) string {
	// Get Internal IP (equivalent to GCE NetworkIP)
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			return addr.Address
		}
	}

	return ""
}
