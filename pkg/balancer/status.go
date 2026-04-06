package balancer

import (
	"FlakyOllama/pkg/models"
	"html/template"
	"net/http"
	"strings"
)

const dashboardTemplate = `
<!DOCTYPE html>
<html>
<head>
    <title>FlakyOllama Status</title>
    <style>
        body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; background: #f0f2f5; margin: 0; padding: 20px; }
        .container { max-width: 1200px; margin: auto; }
        table { width: 100%; border-collapse: collapse; background: #fff; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 5px rgba(0,0,0,0.1); margin-bottom: 30px; }
        th, td { padding: 15px; text-align: left; border-bottom: 1px solid #eee; }
        th { background: #343a40; color: #fff; text-transform: uppercase; font-size: 12px; letter-spacing: 1px; }
        tr:hover { background: #f9f9f9; }
        .state-healthy { color: #28a745; font-weight: bold; }
        .state-degraded { color: #ffc107; font-weight: bold; }
        .state-broken { color: #dc3545; font-weight: bold; }
        .progress-bar { background: #e9ecef; border-radius: 10px; height: 12px; width: 120px; display: inline-block; vertical-align: middle; margin-right: 10px; }
        .progress-fill { background: #007bff; height: 100%; border-radius: 10px; }
        .header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; }
        .badge { background: #6c757d; color: #fff; padding: 4px 8px; border-radius: 4px; font-size: 11px; margin-right: 5px; }
        .card { background: #fff; padding: 20px; border-radius: 8px; box-shadow: 0 2px 5px rgba(0,0,0,0.1); }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>FlakyOllama Node Cluster</h1>
            <div class="card"><strong>Queue Depth:</strong> {{.QueueDepth}} requests</div>
        </div>

        <h2>Active Worker Nodes</h2>
        <table>
            <tr>
                <th>Node ID</th>
                <th>Endpoint</th>
                <th>Status</th>
                <th>Hardware (CPU/VRAM)</th>
                <th>Loaded Models</th>
                <th>Last Seen</th>
            </tr>
            {{range .Nodes}}
            <tr>
                <td><strong>{{.ID}}</strong></td>
                <td><code>{{.Address}}</code></td>
                <td><span class="state-{{.State.String | ToLower}}">{{.State.String}}</span> <small>({{.Errors}} errors)</small></td>
                <td>
                    <div>
                        <div class="progress-bar"><div class="progress-fill" style="width: {{.CPUUsage}}%;"></div></div>
                        <small>{{printf "%.1f" .CPUUsage}}% ({{.CPUCores}} cores)</small>
                    </div>
                    <div style="margin-top: 5px;">
                        <small><strong>GPU:</strong> {{.GPUModel}}</small><br>
                        <small><strong>VRAM:</strong> {{printf "%.1f" (Divide .VRAMUsed 1073741824)}} / {{printf "%.1f" (Divide .VRAMTotal 1073741824)}} GB</small>
                    </div>
                </td>
                <td>
                    {{range .ActiveModels}}<span class="badge">{{.}}</span>{{else}}<small>None</small>{{end}}
                </td>
                <td>{{.LastSeen.Format "15:04:05"}}</td>
            </tr>
            {{end}}
        </table>

        <h2>System Load</h2>
        <div class="card">
            {{range $model, $count := .Pending}}
            <div><strong>{{$model}}:</strong> {{$count}} pending requests</div>
            {{else}}
            <small>No active workloads in the global queue.</small>
            {{end}}
        </div>
    </div>
</body>
</html>
`

func (b *Balancer) HandleStatus(w http.ResponseWriter, r *http.Request) {
	b.Mu.RLock()
	defer b.Mu.RUnlock()

	tmpl := template.New("dashboard").Funcs(template.FuncMap{
		"ToLower": func(s string) string { return strings.ToLower(s) },
		"Divide":  func(a, b uint64) float64 { return float64(a) / float64(b) },
	})
	tmpl, err := tmpl.Parse(dashboardTemplate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := struct {
		Nodes      map[string]*models.NodeStatus
		Pending    map[string]int
		QueueDepth int
	}{
		Nodes:      b.Agents,
		Pending:    b.PendingRequests,
		QueueDepth: b.Queue.pq.Len(),
	}

	w.Header().Set("Content-Type", "text/html")
	tmpl.Execute(w, data)
}
