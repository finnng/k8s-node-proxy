package server

import (
	"k8s-node-proxy/internal/k8s"
)

type ServerInfo struct {
	ProjectID       string
	ClusterName     string
	ClusterLocation string
	K8sEndpoint     string
	NodeIPs         []string
	Services        []k8s.ServiceInfo
}