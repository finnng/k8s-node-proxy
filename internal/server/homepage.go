package server

import (
	"context"
	"html/template"
	"net/http"
	"time"

	"k8s-node-proxy/internal/nodes"
	"k8s-node-proxy/internal/services"
)

const HomepageTemplate = `
<!DOCTYPE html>
<html>
<head>
    <title>k8s-node-proxy</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        table { border-collapse: collapse; width: 100%; margin: 20px 0; }
        th, td { border: 1px solid #ddd; padding: 12px; text-align: left; }
        th { background-color: #f2f2f2; }
        .section { margin: 30px 0; }
        h1 { color: #333; }
        h2 { color: #666; }
        .status-healthy { background-color: #d4edda; padding: 4px 8px; border-radius: 4px; }
        .status-unhealthy { background-color: #f8d7da; padding: 4px 8px; border-radius: 4px; }
        .status-unknown { background-color: #fff3cd; padding: 4px 8px; border-radius: 4px; }
        .info-text { font-size: 12px; color: #666; font-style: italic; margin-top: 8px; line-height: 1.4; }
    </style>
</head>
<body>
    <h1>k8s-node-proxy Server{{if .PlatformName}} ({{.PlatformName}}){{end}}</h1>

    <div class="section">
        <h2>Cluster Information</h2>
        <table>
            <tr><th>Property</th><th>Value</th></tr>
            {{range .ClusterInfo}}
            <tr><td>{{.Key}}</td><td>{{.Value}}</td></tr>
            {{end}}
        </table>
    </div>

    <div class="section">
        <h2>Current Active Node</h2>
        {{if .CurrentNode}}
        <table>
            <tr><th>Property</th><th>Value</th></tr>
            <tr><td>Node Name</td><td>{{.CurrentNode.Name}}</td></tr>
            <tr><td>IP Address</td><td>{{.CurrentNode.IP}}</td></tr>
            <tr><td>Status</td><td>{{.CurrentNode.Status}}</td></tr>
        </table>
        {{else}}
        <p>No current node selected</p>
        {{end}}
        <div class="info-text">
            Node behavior: Health checks every 15 seconds. Failover after 3 consecutive failures to oldest healthy node (max 45 seconds).
            Node list refreshes every 2 minutes for display only - active node remains stable unless unhealthy.
        </div>
    </div>

    <div class="section">
        <h2>All Cluster Nodes</h2>
        <table>
            <tr><th>Node Name</th><th>IP Address</th><th>Status</th><th>Age</th><th>Last Check</th></tr>
            {{range .AllNodes}}
            <tr>
                <td>{{.Name}}</td>
                <td>{{.IP}}</td>
                <td>
                    {{if eq .Status 0}}<span class="status-healthy">Healthy</span>{{else if eq .Status 1}}<span class="status-unhealthy">Unhealthy</span>{{else}}<span class="status-unknown">Unknown</span>{{end}}
                </td>
                <td>{{printf "%.0f" .Age.Hours}}h</td>
                <td>{{.LastCheck.Format "15:04:05"}}</td>
            </tr>
            {{end}}
        </table>
    </div>

    <div class="section">
        <h2>NodePort Services ({{.Namespace}} namespace)</h2>
        <table>
            <tr><th>Service</th><th>Namespace</th><th>NodePort</th><th>TargetPort</th><th>Protocol</th></tr>
            {{range .Services}}
            <tr>
                <td>{{.Name}}</td>
                <td>{{.Namespace}}</td>
                <td>{{.NodePort}}</td>
                <td>{{.TargetPort}}</td>
                <td>{{.Protocol}}</td>
            </tr>
            {{end}}
        </table>
    </div>

    <div class="section">
        <p><strong>Proxy Status:</strong> Active and forwarding traffic to current cluster nodes</p>
        <p><strong>Health Check:</strong> <a href="/health">/health</a></p>
    </div>
</body>
</html>
`

type ClusterInfoField struct {
	Key   string
	Value string
}

type CurrentNodeInfo struct {
	Name   string
	IP     string
	Status string
}

type HomepageData struct {
	PlatformName string
	ClusterInfo  []ClusterInfoField
	Namespace    string
	CurrentNode  *CurrentNodeInfo
	AllNodes     []nodes.NodeInfo
	Services     []services.ServiceInfo
}

func (s *Server) handleHomepage(w http.ResponseWriter, r *http.Request) {
	if s.serverInfo == nil {
		http.Error(w, "Server info not yet collected", http.StatusServiceUnavailable)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	allNodes, err := s.nodeIPDiscovery.GetAllNodes(ctx)
	if err != nil {
		http.Error(w, "Failed to get current node data", http.StatusInternalServerError)
		return
	}

	currentNodeName := s.nodeIPDiscovery.GetCurrentNodeName()
	currentNodeIP, _ := s.nodeIPDiscovery.GetCurrentNodeIP(ctx)

	var currentNodeInfo *CurrentNodeInfo
	if currentNodeName != "" {
		currentNodeInfo = &CurrentNodeInfo{
			Name:   currentNodeName,
			IP:     currentNodeIP,
			Status: "healthy",
		}
	}

	clusterInfo := []ClusterInfoField{
		{Key: "Project ID", Value: s.serverInfo.ProjectID},
		{Key: "Cluster Name", Value: s.serverInfo.ClusterName},
		{Key: "Cluster Location", Value: s.serverInfo.ClusterLocation},
		{Key: "Kubernetes Endpoint", Value: s.serverInfo.K8sEndpoint},
		{Key: "Target Namespace", Value: s.serverInfo.Namespace},
	}

	data := HomepageData{
		PlatformName: "GKE",
		ClusterInfo:  clusterInfo,
		Namespace:    s.serverInfo.Namespace,
		CurrentNode:  currentNodeInfo,
		AllNodes:     allNodes,
		Services:     s.serverInfo.Services,
	}

	tmpl, err := template.New("homepage").Parse(HomepageTemplate)
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	if err := tmpl.Execute(w, &data); err != nil {
		http.Error(w, "Template execution error", http.StatusInternalServerError)
		return
	}
}