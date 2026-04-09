package balancer

import (
	"FlakyOllama/pkg/models"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"
	"time"
)

const dashboardTemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>FlakyOllama | Management Console</title>
    <script src="https://unpkg.com/htmx.org@1.9.10"></script>
    <script src="https://cdn.tailwindcss.com"></script>
    <style>
        @import url('https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap');
        body { font-family: 'Inter', sans-serif; }
        .progress-bar-bg { background-color: #e5e7eb; border-radius: 9999px; height: 0.5rem; overflow: hidden; }
        .progress-bar-fill { height: 100%; transition: width 0.5s ease-out; }
        .htmx-indicator { display: none; }
        .htmx-request .htmx-indicator { display: inline; }
        .htmx-request.htmx-indicator { display: inline; }
    </style>
</head>
<body class="bg-gray-50 text-gray-900 min-h-screen">
    <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
        <!-- Header -->
        <header class="flex flex-col md:flex-row md:items-center md:justify-between mb-8 gap-4">
            <div>
                <h1 class="text-3xl font-bold tracking-tight text-gray-900 flex items-center gap-2">
                    <span class="text-indigo-600">Flaky</span>Ollama
                    <span class="bg-indigo-100 text-indigo-700 text-xs font-semibold px-2.5 py-0.5 rounded-full">v1.0.0</span>
                </h1>
                <p class="mt-1 text-sm text-gray-500">Intelligent cluster management for unreliable Ollama nodes.</p>
            </div>
            <div class="flex items-center gap-4">
                <div class="bg-white px-4 py-2 rounded-lg shadow-sm border border-gray-200 flex items-center gap-3">
                    <div class="p-2 bg-amber-50 rounded-md">
                        <svg class="w-5 h-5 text-amber-600" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"></path></svg>
                    </div>
                    <div>
                        <p class="text-xs text-gray-500 uppercase font-semibold">Queue Depth</p>
                        <p class="text-lg font-bold text-gray-900" id="queue-depth-val">{{.QueueDepth}}</p>
                    </div>
                </div>
                <button hx-get="/status" hx-target="body" class="p-2 text-gray-500 hover:text-indigo-600 transition-colors">
                    <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"></path></svg>
                </button>
            </div>
        </header>

        <!-- Main Dashboard -->
        <div id="dashboard-content" hx-get="/status?partial=true" hx-trigger="every 5s" hx-swap="innerHTML">
            {{template "partial" .}}
        </div>

        <!-- Playground & Global Models -->
        <div class="mt-12 grid grid-cols-1 lg:grid-cols-3 gap-8">
            <!-- Test Section -->
            <div class="lg:col-span-2 space-y-8">
                <div class="bg-white rounded-xl shadow-sm border border-gray-200 overflow-hidden">
                    <div class="px-6 py-4 border-b border-gray-100 bg-gray-50/50 flex justify-between items-center">
                        <h2 class="text-lg font-semibold text-gray-900 flex items-center gap-2">
                            <svg class="w-5 h-5 text-indigo-500" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z"></path><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path></svg>
                            Cluster Playground
                        </h2>
                    </div>
                    <div class="p-6">
                        <form hx-post="/api/manage/test" hx-target="#test-result" hx-indicator="#test-spinner" class="space-y-4">
                            <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
                                <div>
                                    <label class="block text-sm font-medium text-gray-700 mb-1">Target Model</label>
                                    <select name="model" class="w-full rounded-md border-gray-300 shadow-sm focus:border-indigo-500 focus:ring-indigo-500 sm:text-sm">
                                        {{range .AllModels}}
                                        <option value="{{.}}">{{.}}</option>
                                        {{else}}
                                        <option disabled>No models available</option>
                                        {{end}}
                                    </select>
                                </div>
                            </div>
                            <div>
                                <label class="block text-sm font-medium text-gray-700 mb-1">Test Prompt</label>
                                <textarea name="prompt" rows="3" class="w-full rounded-md border-gray-300 shadow-sm focus:border-indigo-500 focus:ring-indigo-500 sm:text-sm" placeholder="Ask something to test the cluster routing..."></textarea>
                            </div>
                            <button type="submit" class="inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md shadow-sm text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 transition-colors">
                                Run Inference Test
                                <svg id="test-spinner" class="animate-spin ml-2 h-4 w-4 text-white htmx-indicator" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path></svg>
                            </button>
                        </form>

                        <div id="test-result" class="mt-6">
                            <div class="text-xs text-gray-400 italic">Results will appear here...</div>
                        </div>
                    </div>
                </div>
            </div>

            <!-- Model Management Sidebar -->
            <div class="space-y-8">
                <!-- Pull New Model -->
                <div class="bg-white rounded-xl shadow-sm border border-gray-200 overflow-hidden">
                    <div class="px-6 py-4 border-b border-gray-100 bg-gray-50/50">
                        <h2 class="text-lg font-semibold text-gray-900 flex items-center gap-2">
                            <svg class="w-5 h-5 text-indigo-500" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M9 19l3 3m0 0l3-3m-3 3V10"></path></svg>
                            Pull New Model
                        </h2>
                    </div>
                    <div class="p-6">
                        <form hx-post="/api/manage/model/pull" hx-target="#pull-status" hx-indicator="#pull-spinner" class="space-y-4">
                            <div>
                                <label class="block text-sm font-medium text-gray-700 mb-1">Model Name</label>
                                <input type="text" name="model" placeholder="e.g. llama3, mistral" class="w-full rounded-md border-gray-300 shadow-sm focus:border-indigo-500 focus:ring-indigo-500 sm:text-sm">
                                <p class="mt-1 text-xs text-gray-500 italic">This will trigger a pull on all active nodes.</p>
                            </div>
                            <button type="submit" class="w-full inline-flex justify-center items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md shadow-sm text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 transition-colors">
                                Pull to Cluster
                                <svg id="pull-spinner" class="animate-spin ml-2 h-4 w-4 text-white htmx-indicator" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path></svg>
                            </button>
                        </form>
                        <div id="pull-status" class="mt-4"></div>
                    </div>
                </div>

                <!-- Global Model Catalog -->
                <div class="bg-white rounded-xl shadow-sm border border-gray-200 overflow-hidden">
                    <div class="px-6 py-4 border-b border-gray-100 bg-gray-50/50">
                        <h2 class="text-lg font-semibold text-gray-900 flex items-center gap-2">
                            <svg class="w-5 h-5 text-emerald-500" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"></path></svg>
                            Global Model Catalog
                        </h2>
                    </div>
                    <div class="divide-y divide-gray-100 max-h-[400px] overflow-y-auto">
                        {{range .AllModels}}
                        <div class="px-6 py-3 flex justify-between items-center hover:bg-gray-50/50">
                            <div>
                                <div class="text-sm font-medium text-gray-700">{{.}}</div>
                            </div>
                            <span class="inline-flex items-center px-2 py-0.5 rounded text-[10px] font-bold bg-emerald-100 text-emerald-700 uppercase">Available</span>
                        </div>
                        {{else}}
                        <div class="p-6 text-center text-gray-400 italic text-sm">No models registered in the cluster.</div>
                        {{end}}
                    </div>
                </div>
            </div>
        </div>
    </div>

    <!-- Modals / Toast Area -->
    <div id="notifications" class="fixed bottom-4 right-4 z-50"></div>
</body>
</html>
`

const partialTemplate = `
{{define "partial"}}
    <!-- Stats Grid -->
    <div class="grid grid-cols-1 md:grid-cols-3 gap-6 mb-8">
        <div class="bg-white p-6 rounded-xl shadow-sm border border-gray-200">
            <h3 class="text-sm font-medium text-gray-500 mb-1">Total Nodes</h3>
            <div class="flex items-end gap-2">
                <span class="text-3xl font-bold text-gray-900">{{len .Nodes}}</span>
                <span class="text-sm text-green-600 font-medium mb-1">Online</span>
            </div>
        </div>
        <div class="bg-white p-6 rounded-xl shadow-sm border border-gray-200">
            <h3 class="text-sm font-medium text-gray-500 mb-1">Active Workloads</h3>
            <div class="flex items-end gap-2">
                <span class="text-3xl font-bold text-gray-900">{{.ActiveWorkloads}}</span>
                <span class="text-sm text-indigo-600 font-medium mb-1">Pending</span>
            </div>
        </div>
        <div class="bg-white p-6 rounded-xl shadow-sm border border-gray-200">
            <h3 class="text-sm font-medium text-gray-500 mb-1">Cluster Health</h3>
            <div class="flex items-center gap-2 mt-1">
                <span class="h-3 w-3 rounded-full bg-green-500 animate-pulse"></span>
                <span class="text-lg font-semibold text-gray-900">Operational</span>
            </div>
        </div>
    </div>

    <!-- Node Table -->
    <div class="bg-white rounded-xl shadow-sm border border-gray-200 overflow-hidden">
        <div class="px-6 py-4 border-b border-gray-100 flex justify-between items-center bg-gray-50/50">
            <h2 class="text-lg font-semibold text-gray-900">Worker Nodes</h2>
            <span class="text-xs text-gray-400">Last updated: {{now}}</span>
        </div>
        <div class="overflow-x-auto">
            <table class="w-full text-left">
                <thead class="bg-gray-50/50 text-gray-500 text-xs uppercase tracking-wider">
                    <tr>
                        <th class="px-6 py-4 font-semibold">Node Identity</th>
                        <th class="px-6 py-4 font-semibold">Health & Status</th>
                        <th class="px-6 py-4 font-semibold">Hardware Utilization</th>
                        <th class="px-6 py-4 font-semibold">Loaded / Stored Models</th>
                        <th class="px-6 py-4 font-semibold">Actions</th>
                    </tr>
                </thead>
                <tbody class="divide-y divide-gray-100">
                    {{range .Nodes}}
                    <tr class="hover:bg-gray-50/50 transition-colors">
                        <td class="px-6 py-4 align-top">
                            <div class="font-bold text-gray-900">{{.ID}}</div>
                            <div class="text-sm text-gray-500 font-mono">{{.Address}}</div>
                        </td>
                        <td class="px-6 py-4 align-top">
                            <div class="flex items-center gap-2">
                                <span class="h-2 w-2 rounded-full {{if eq .State.String "Healthy"}}bg-green-500{{else if eq .State.String "Degraded"}}bg-amber-500{{else}}bg-red-500{{end}}"></span>
                                <span class="font-medium {{if eq .State.String "Healthy"}}text-green-700{{else if eq .State.String "Degraded"}}text-amber-700{{else}}text-red-700{{end}}">
                                    {{.State.String}}
                                </span>
                            </div>
                            <div class="text-xs text-gray-400 mt-1">{{.Errors}} errors • Seen {{since .LastSeen}}</div>
                            {{if .Draining}}
                            <span class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-amber-100 text-amber-800 mt-2">
                                <svg class="w-3 h-3 mr-1" fill="currentColor" viewBox="0 0 20 20"><path fill-rule="evenodd" d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z" clip-rule="evenodd"></path></svg>
                                Draining
                            </span>
                            {{end}}
                        </td>
                        <td class="px-6 py-4 align-top min-w-[200px]">
                            <!-- CPU -->
                            <div class="mb-3">
                                <div class="flex justify-between text-xs mb-1">
                                    <span class="text-gray-500">CPU Usage</span>
                                    <span class="font-medium text-gray-900">{{printf "%.1f" .CPUUsage}}%</span>
                                </div>
                                <div class="progress-bar-bg">
                                    <div class="progress-bar-fill {{if gt .CPUUsage 85.0}}bg-red-500{{else if gt .CPUUsage 60.0}}bg-amber-500{{else}}bg-indigo-500{{end}}" style="width: {{.CPUUsage}}%;"></div>
                                </div>
                            </div>
                            <!-- VRAM -->
                            <div class="mb-3">
                                <div class="flex justify-between text-xs mb-1">
                                    <span class="text-gray-500">VRAM ({{.GPUModel}})</span>
                                    <span class="font-medium text-gray-900">{{printf "%.1f" (Divide .VRAMUsed 1073741824)}} / {{printf "%.1f" (Divide .VRAMTotal 1073741824)}} GB</span>
                                </div>
                                <div class="progress-bar-bg">
                                    <div class="progress-bar-fill bg-emerald-500" style="width: {{Percentage .VRAMUsed .VRAMTotal}}%;"></div>
                                </div>
                            </div>
                            <!-- Temp -->
                            <div class="flex items-center gap-2 text-xs text-gray-500">
                                <svg class="w-4 h-4 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z"></path></svg>
                                <span class="{{if gt .GPUTemperature 80.0}}text-red-600 font-bold{{else if gt .GPUTemperature 70.0}}text-amber-600{{else}}text-green-600{{end}}">
                                    {{printf "%.1f" .GPUTemperature}}°C
                                </span>
                            </div>
                        </td>
                        <td class="px-6 py-4 align-top">
                            <div class="space-y-4">
                                <!-- Loaded Models (In Memory) -->
                                <div>
                                    <p class="text-[10px] font-bold text-gray-400 uppercase mb-1 tracking-wider">In Memory</p>
                                    <div class="flex flex-wrap gap-1.5 max-w-[250px]">
                                        {{range .ActiveModels}}
                                        <div class="group relative inline-flex items-center px-2 py-0.5 rounded text-[10px] font-medium bg-indigo-100 text-indigo-800 border border-indigo-200">
                                            {{.}}
                                            <button hx-post="/api/manage/model/unload?id={{$.ID}}&model={{.}}" hx-target="#dashboard-content" hx-swap="innerHTML" title="Unload from memory" class="ml-1 text-indigo-400 hover:text-red-500 transition-colors">
                                                <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path></svg>
                                            </button>
                                        </div>
                                        {{else}}
                                        <span class="text-[10px] text-gray-400 italic">None</span>
                                        {{end}}
                                    </div>
                                </div>
                                <!-- Local Models (On Disk) -->
                                <div>
                                    <p class="text-[10px] font-bold text-gray-400 uppercase mb-1 tracking-wider">On Disk</p>
                                    <div class="flex flex-wrap gap-1.5 max-w-[250px]">
                                        {{range .LocalModels}}
                                        <div class="group relative inline-flex items-center px-2 py-0.5 rounded text-[10px] font-medium bg-gray-100 text-gray-800 border border-gray-200">
                                            {{.Name}}
                                            <button hx-post="/api/manage/model/delete?id={{$.ID}}&model={{.Name}}" hx-confirm="Are you sure you want to delete '{{.Name}}' from this node's disk?" hx-target="#dashboard-content" hx-swap="innerHTML" title="Delete from disk" class="ml-1 text-gray-400 hover:text-red-500 transition-colors opacity-0 group-hover:opacity-100">
                                                <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"></path></svg>
                                            </button>
                                        </div>
                                        {{else}}
                                        <span class="text-[10px] text-gray-400 italic">None</span>
                                        {{end}}
                                    </div>
                                </div>
                            </div>
                        </td>
                        <td class="px-6 py-4 align-top text-sm">
                            <div class="flex flex-col gap-2">
                                {{if .Draining}}
                                <button hx-post="/api/manage/node/undrain?id={{.ID}}" hx-target="#dashboard-content" hx-swap="innerHTML" class="w-full px-3 py-1.5 font-semibold rounded-md bg-white text-gray-700 border border-gray-300 hover:bg-gray-50 transition-colors">
                                    Undrain
                                </button>
                                {{else}}
                                <button hx-post="/api/manage/node/drain?id={{.ID}}" hx-target="#dashboard-content" hx-swap="innerHTML" class="w-full px-3 py-1.5 font-semibold rounded-md bg-amber-50 text-amber-700 border border-amber-200 hover:bg-amber-100 transition-colors">
                                    Drain
                                </button>
                                {{end}}
                                <button hx-post="/api/manage/model/pull?id={{.ID}}" hx-prompt="Enter model name to pull to this node:" hx-target="#pull-status-{{.ID}}" class="w-full px-3 py-1.5 text-xs font-semibold rounded-md bg-indigo-50 text-indigo-700 border border-indigo-200 hover:bg-indigo-100 transition-colors">
                                    Pull Model
                                </button>
                                <div id="pull-status-{{.ID}}" class="mt-1"></div>
                            </div>
                        </td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>
    </div>

    <!-- Active Queues -->
    <div class="mt-8">
        <h3 class="text-lg font-semibold text-gray-900 mb-4">Live Request Traffic</h3>
        <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {{range $model, $count := .Pending}}
            <div class="bg-white p-4 rounded-lg shadow-sm border border-gray-200 flex justify-between items-center">
                <div class="flex items-center gap-3">
                    <div class="w-2 h-2 rounded-full bg-indigo-500 animate-pulse"></div>
                    <span class="font-medium text-gray-900">{{$model}}</span>
                </div>
                <span class="bg-indigo-50 text-indigo-700 text-xs font-bold px-2 py-1 rounded">
                    {{$count}} pending
                </span>
            </div>
            {{else}}
            <div class="col-span-full py-8 text-center bg-gray-50 rounded-lg border-2 border-dashed border-gray-200">
                <p class="text-gray-400">No active workloads in the global queue.</p>
            </div>
            {{end}}
        </div>
    </div>
{{end}}
`

func (b *Balancer) HandleStatus(w http.ResponseWriter, r *http.Request) {
	b.Mu.RLock()
	defer b.Mu.RUnlock()

	tmpl := template.New("dashboard").Funcs(template.FuncMap{
		"ToLower": func(s string) string { return strings.ToLower(s) },
		"Divide":  func(a, b uint64) float64 { return float64(a) / float64(b) },
		"Percentage": func(a, b uint64) float64 {
			if b == 0 {
				return 0
			}
			return (float64(a) / float64(b)) * 100
		},
		"since": func(t time.Time) string {
			if t.IsZero() {
				return "never"
			}
			d := time.Since(t).Round(time.Second)
			if d < time.Minute {
				return fmt.Sprintf("%v", d)
			}
			return fmt.Sprintf("%v", d.Round(time.Minute))
		},
		"now": func() string { return time.Now().Format("15:04:05") },
	})

	var err error
	tmpl, err = tmpl.Parse(dashboardTemplate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl, err = tmpl.Parse(partialTemplate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	totalWorkloads := 0
	for _, count := range b.PendingRequests {
		totalWorkloads += count
	}

	modelMap := make(map[string]bool)
	for _, agent := range b.Agents {
		for _, model := range agent.LocalModels {
			modelMap[model.Name] = true
		}
		for _, model := range agent.ActiveModels {
			modelMap[model] = true
		}
	}
	allModels := make([]string, 0, len(modelMap))
	for m := range modelMap {
		allModels = append(allModels, m)
	}
	sort.Strings(allModels)

	data := struct {
		Nodes           map[string]*models.NodeStatus
		Pending         map[string]int
		QueueDepth      int
		ActiveWorkloads int
		AllModels       []string
	}{
		Nodes:           b.Agents,
		Pending:         b.PendingRequests,
		QueueDepth:      b.Queue.pq.Len(),
		ActiveWorkloads: totalWorkloads,
		AllModels:       allModels,
	}

	w.Header().Set("Content-Type", "text/html")
	if r.URL.Query().Get("partial") == "true" {
		tmpl.ExecuteTemplate(w, "partial", data)
	} else {
		tmpl.Execute(w, data)
	}
}
