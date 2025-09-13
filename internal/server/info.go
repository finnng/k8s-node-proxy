package server

import (
	"k8s-node-proxy/internal/discovery"
	"k8s-node-proxy/internal/k8s"
)

type ServerInfo struct {
	ProjectID       string
	ClusterName     string
	ClusterLocation string
	K8sEndpoint     string
	NodeIPs         []string
	Services        []k8s.ServiceInfo
	CurrentNode     *CurrentNodeInfo
	AllNodes        []discovery.NodeInfo
}

type CurrentNodeInfo struct {
	Name   string
	IP     string
	Status string
}