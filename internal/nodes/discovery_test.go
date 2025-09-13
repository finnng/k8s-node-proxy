package nodes

import (
	"testing"
	"time"
)

// TestNodeStatus tests the NodeStatus enum values
func TestNodeStatus_Values(t *testing.T) {
	tests := []struct {
		name     string
		status   NodeStatus
		expected NodeStatus
	}{
		{"NodeHealthy", NodeHealthy, 0},
		{"NodeUnhealthy", NodeUnhealthy, 1},
		{"NodeUnknown", NodeUnknown, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.status != tt.expected {
				t.Errorf("Expected %s to have value %d, got %d", tt.name, tt.expected, tt.status)
			}
		})
	}
}

// TestNodeSelection tests node selection logic with test data
func TestNodeSelection_GetOldestHealthyNode(t *testing.T) {
	nodes := []NodeInfo{
		{
			Name:         "node-1",
			IP:           "10.0.1.1",
			Status:       NodeHealthy,
			CreationTime: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			Name:         "node-2",
			IP:           "10.0.1.2",
			Status:       NodeHealthy,
			CreationTime: time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC),
		},
		{
			Name:         "node-3",
			IP:           "10.0.1.3",
			Status:       NodeUnhealthy,
			CreationTime: time.Date(2022, 12, 31, 0, 0, 0, 0, time.UTC), // Oldest but unhealthy
		},
	}

	// Should return node-1 (oldest healthy node)
	oldestHealthy := getOldestHealthyNode(nodes)
	if oldestHealthy == nil {
		t.Fatal("Expected to find healthy node, got nil")
	}
	if oldestHealthy.Name != "node-1" {
		t.Errorf("Expected node-1, got %s", oldestHealthy.Name)
	}
	if oldestHealthy.IP != "10.0.1.1" {
		t.Errorf("Expected IP 10.0.1.1, got %s", oldestHealthy.IP)
	}
}

func TestNodeSelection_NoHealthyNodes(t *testing.T) {
	nodes := []NodeInfo{
		{
			Name:   "node-1",
			IP:     "10.0.1.1",
			Status: NodeUnhealthy,
		},
		{
			Name:   "node-2",
			IP:     "10.0.1.2",
			Status: NodeUnknown,
		},
	}

	oldestHealthy := getOldestHealthyNode(nodes)
	if oldestHealthy != nil {
		t.Errorf("Expected no healthy nodes, got %+v", oldestHealthy)
	}
}

// Helper function to test node selection logic
func getOldestHealthyNode(nodes []NodeInfo) *NodeInfo {
	var oldestHealthy *NodeInfo

	for i := range nodes {
		node := &nodes[i]
		if node.Status != NodeHealthy {
			continue
		}

		if oldestHealthy == nil || node.CreationTime.Before(oldestHealthy.CreationTime) {
			oldestHealthy = node
		}
	}

	return oldestHealthy
}
