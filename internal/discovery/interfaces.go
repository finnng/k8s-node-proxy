// Package discovery defines interfaces for service and node discoverypackage discovery

package discovery

import (
	"context"
	"time"
)

// ServiceInfo represents a discovered service
type ServiceInfo struct {
	Name       string
	Namespace  string
	NodePort   int32
	TargetPort int32
	Protocol   string
}

// ClusterInfo represents cluster information
type ClusterInfo struct {
	Name     string
	Location string
	Endpoint string
}

// NodeStatus represents the health status of a node
type NodeStatus int

const (
	NodeHealthy NodeStatus = iota
	NodeUnhealthy
	NodeUnknown
)

// NodeInfo represents a discovered node
type NodeInfo struct {
	Name         string
	IP           string
	Status       NodeStatus
	Age          time.Duration
	CreationTime time.Time
	LastCheck    time.Time
}

// ServiceDiscovery interface for discovering services
type ServiceDiscovery interface {
	DiscoverNodePorts(ctx context.Context) ([]int, error)
	DiscoverServices(ctx context.Context) ([]ServiceInfo, error)
	GetClusterInfo() *ClusterInfo
}

// NodeDiscovery interface for discovering nodes
type NodeDiscovery interface {
	GetCurrentNodeIP(ctx context.Context) (string, error)
	GetAllNodes(ctx context.Context) ([]NodeInfo, error)
	StartHealthMonitoring()
	StopHealthMonitoring()
	GetCurrentNodeName() string
}
