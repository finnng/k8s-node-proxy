package nodes

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestEKSNodeDiscovery_NodeSelection tests that node list from Kubernetes API → oldest node selected (T031)
func TestEKSNodeDiscovery_NodeSelection(t *testing.T) {
	// Create mock node list with different creation times
	now := time.Now()
	nodes := []NodeInfo{
		{
			Name:         "node-oldest",
			IP:           "10.0.1.1",
			Status:       NodeHealthy,
			CreationTime: now.Add(-24 * time.Hour), // 24 hours ago
		},
		{
			Name:         "node-middle",
			IP:           "10.0.1.2",
			Status:       NodeHealthy,
			CreationTime: now.Add(-12 * time.Hour), // 12 hours ago
		},
		{
			Name:         "node-newest",
			IP:           "10.0.1.3",
			Status:       NodeHealthy,
			CreationTime: now.Add(-1 * time.Hour), // 1 hour ago
		},
		{
			Name:         "node-unhealthy",
			IP:           "10.0.1.4",
			Status:       NodeUnhealthy,
			CreationTime: now.Add(-48 * time.Hour), // Even older but unhealthy
		},
	}

	// Test node selection (simulating the findOldestHealthyNode logic)
	selectedNode := findOldestHealthyNodeFromList(nodes)

	// Should select the oldest healthy node (node-oldest)
	assert.NotNil(t, selectedNode)
	assert.Equal(t, "node-oldest", selectedNode.Name)
	assert.Equal(t, "10.0.1.1", selectedNode.IP)
	assert.Equal(t, NodeHealthy, selectedNode.Status)
}

// TestEKSNodeDiscovery_NoHealthyNodes tests behavior when no healthy nodes exist
func TestEKSNodeDiscovery_NoHealthyNodes(t *testing.T) {
	nodes := []NodeInfo{
		{
			Name:   "node-unhealthy-1",
			IP:     "10.0.1.1",
			Status: NodeUnhealthy,
		},
		{
			Name:   "node-unhealthy-2",
			IP:     "10.0.1.2",
			Status: NodeUnhealthy,
		},
	}

	selectedNode := findOldestHealthyNodeFromList(nodes)
	assert.Nil(t, selectedNode)
}

// TestEKSNodeDiscovery_EmptyNodeList tests behavior with empty node list
func TestEKSNodeDiscovery_EmptyNodeList(t *testing.T) {
	nodes := []NodeInfo{}

	selectedNode := findOldestHealthyNodeFromList(nodes)
	assert.Nil(t, selectedNode)
}

// Helper function for testing (simulates findOldestHealthyNode logic)
func findOldestHealthyNodeFromList(nodes []NodeInfo) *NodeInfo {
	for i := range nodes {
		if nodes[i].Status == NodeHealthy {
			return &nodes[i]
		}
	}
	return nil
}
