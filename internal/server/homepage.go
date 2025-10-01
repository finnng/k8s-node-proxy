package server

import (
	"html/template"
	"net/http"
)

const homepageTemplate = `
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
    <h1>k8s-node-proxy Server</h1>
    
    <div class="section">
        <h2>Cluster Information</h2>
        <table>
            <tr><th>Property</th><th>Value</th></tr>
            <tr><td>Project ID</td><td>{{.ProjectID}}</td></tr>
            <tr><td>Cluster Name</td><td>{{.ClusterName}}</td></tr>
            <tr><td>Cluster Location</td><td>{{.ClusterLocation}}</td></tr>
            <tr><td>Kubernetes Endpoint</td><td>{{.K8sEndpoint}}</td></tr>
            <tr><td>Target Namespace</td><td>{{.Namespace}}</td></tr>
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

func (s *Server) handleHomepage(w http.ResponseWriter, r *http.Request) {
	if s.serverInfo == nil {
		http.Error(w, "Server info not yet collected", http.StatusServiceUnavailable)
		return
	}

	// Get fresh node data for display
	ctx := r.Context()
	allNodes, err := s.nodeIPDiscovery.GetAllNodes(ctx)
	if err != nil {
		http.Error(w, "Failed to get current node data", http.StatusInternalServerError)
		return
	}

	// Get current node info
	currentNodeName := s.nodeIPDiscovery.GetCurrentNodeName()
	currentNodeIP, _ := s.nodeIPDiscovery.GetCurrentNodeIP(ctx)

	var currentNodeInfo *CurrentNodeInfo
	if currentNodeName != "" {
		currentNodeInfo = &CurrentNodeInfo{
			Name:   currentNodeName,
			IP:     currentNodeIP,
			Status: "healthy", // Will be updated by health monitoring
		}
	}

	// Create fresh server info with updated node data
	freshServerInfo := *s.serverInfo
	freshServerInfo.AllNodes = allNodes
	freshServerInfo.CurrentNode = currentNodeInfo

	tmpl, err := template.New("homepage").Parse(homepageTemplate)
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	if err := tmpl.Execute(w, &freshServerInfo); err != nil {
		http.Error(w, "Template execution error", http.StatusInternalServerError)
		return
	}
}