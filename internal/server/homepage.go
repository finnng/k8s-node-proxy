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
        </table>
    </div>

    <div class="section">
        <h2>Node IPs</h2>
        <table>
            <tr><th>Node IP</th></tr>
            {{range .NodeIPs}}
            <tr><td>{{.}}</td></tr>
            {{end}}
        </table>
    </div>

    <div class="section">
        <h2>NodePort Services</h2>
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

	tmpl, err := template.New("homepage").Parse(homepageTemplate)
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	if err := tmpl.Execute(w, s.serverInfo); err != nil {
		http.Error(w, "Template execution error", http.StatusInternalServerError)
		return
	}
}