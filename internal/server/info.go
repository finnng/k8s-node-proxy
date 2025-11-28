package server

import (
	"k8s-node-proxy/internal/nodes"
	"k8s-node-proxy/internal/services"
)

type ServerInfo struct {
	ProjectID       string
	ClusterName     string
	ClusterLocation string
	K8sEndpoint     string
	Namespace       string
	NodeIPs         []string
	Services        []services.ServiceInfo
	CurrentNode     *CurrentNodeInfo
	AllNodes        []nodes.NodeInfo
}