package nodes

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestGenericNodeDiscovery_NodeSelection tests that node list from Kubernetes API → oldest node selected (T033)
func TestGenericNodeDiscovery_NodeSelection(t *testing.T) {
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

	// Create a GenericNodeDiscovery instance to test the method
	discovery := &GenericNodeDiscovery{}

	// Test node selection
	selectedNode := discovery.findOldestHealthyNode(nodes)

	// Should select the oldest healthy node (node-oldest)
	assert.NotNil(t, selectedNode)
	assert.Equal(t, "node-oldest", selectedNode.Name)
	assert.Equal(t, "10.0.1.1", selectedNode.IP)
	assert.Equal(t, NodeHealthy, selectedNode.Status)
}

// TestGenericNodeDiscovery_NoHealthyNodes tests behavior when no healthy nodes are available (T034)
func TestGenericNodeDiscovery_NoHealthyNodes(t *testing.T) {
	nodes := []NodeInfo{
		{
			Name:   "node-1",
			IP:     "10.0.1.1",
			Status: NodeUnhealthy,
		},
		{
			Name:   "node-2",
			IP:     "10.0.1.2",
			Status: NodeUnhealthy,
		},
	}

	discovery := &GenericNodeDiscovery{}
	selectedNode := discovery.findOldestHealthyNode(nodes)

	// Should return nil when no healthy nodes
	assert.Nil(t, selectedNode)
}

// TestGenericNodeDiscovery_EmptyNodeList tests behavior with empty node list (T035)
func TestGenericNodeDiscovery_EmptyNodeList(t *testing.T) {
	var nodes []NodeInfo

	discovery := &GenericNodeDiscovery{}
	selectedNode := discovery.findOldestHealthyNode(nodes)

	// Should return nil with empty list
	assert.Nil(t, selectedNode)
}
