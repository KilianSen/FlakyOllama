import React, { useState, useEffect } from 'react';
import { api } from './api';
import type { ClusterStatus } from './api';
import { 
  Server, 
  Database, 
  Thermometer, 
  Trash2, 
  XCircle,
  Play,
  Layers,
  RefreshCw
} from 'lucide-react';

const StateLabel = ({ state }: { state: number }) => {
  const states = ['Healthy', 'Degraded', 'Broken'];
  const colors = ['bg-green-500', 'bg-amber-500', 'bg-red-500'];
  const textColors = ['text-green-700', 'text-amber-700', 'text-red-700'];
  return (
    <div className="flex items-center gap-2">
      <span className={`h-2 w-2 rounded-full ${colors[state] || 'bg-gray-400'}`}></span>
      <span className={`font-medium ${textColors[state] || 'text-gray-400'}`}>{states[state] || 'Unknown'}</span>
    </div>
  );
};

const ProgressBar = ({ value, label, sublabel, colorClass = "bg-indigo-500" }: { value: number, label: string, sublabel?: string, colorClass?: string }) => (
  <div className="mb-3">
    <div className="flex justify-between text-xs mb-1">
      <span className="text-gray-500">{label}</span>
      <span className="font-medium text-gray-900">{sublabel || `${value.toFixed(1)}%`}</span>
    </div>
    <div className="bg-gray-200 rounded-full h-2 overflow-hidden">
      <div 
        className={`h-full transition-all duration-500 ${colorClass}`} 
        style={{ width: `${Math.min(100, Math.max(0, value))}%` }}
      ></div>
    </div>
  </div>
);

const App = () => {
  const [status, setStatus] = useState<ClusterStatus | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [testResult, setTestResult] = useState<{agent_id: string, response: string} | null>(null);
  const [testLoading, setTestLoading] = useState(false);

  const fetchStatus = async () => {
    try {
      const data = await api.getStatus();
      setStatus(data);
      setError(null);
    } catch (err) {
      setError('Connection lost to balancer');
    }
  };

  useEffect(() => {
    fetchStatus();
    const interval = setInterval(fetchStatus, 5000);
    return () => clearInterval(interval);
  }, []);

  const handleDrain = async (addr: string, draining: boolean) => {
    if (draining) await api.undrainNode(addr);
    else await api.drainNode(addr);
    fetchStatus();
  };

  const handleUnload = async (addr: string, model: string) => {
    await api.unloadModel(addr, model);
    fetchStatus();
  };

  const handleDelete = async (addr: string, model: string) => {
    if (window.confirm(`Delete ${model} from ${addr}?`)) {
      await api.deleteModel(addr, model);
      fetchStatus();
    }
  };

  const handlePullNode = async (addr: string) => {
    const model = window.prompt('Enter model name to pull:');
    if (model) {
      await api.pullModel(model, addr);
      fetchStatus();
    }
  };

  const handlePullCluster = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const formData = new FormData(e.currentTarget);
    const model = formData.get('model') as string;
    if (model) {
      await api.pullModel(model);
      alert('Pull triggered for cluster');
      (e.target as HTMLFormElement).reset();
    }
  };

  const handleTest = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const formData = new FormData(e.currentTarget);
    const model = formData.get('model') as string;
    const prompt = formData.get('prompt') as string;
    if (model && prompt) {
      setTestLoading(true);
      try {
        const res = await api.runTest(model, prompt);
        setTestResult(res);
      } catch (err) {
        alert('Test failed');
      } finally {
        setTestLoading(false);
      }
    }
  };

  if (!status) return (
    <div className="flex items-center justify-center min-h-screen">
      <div className="text-center">
        <RefreshCw className="w-12 h-12 text-indigo-500 animate-spin mx-auto mb-4" />
        <p className="text-gray-500 font-medium">{error || 'Connecting to cluster...'}</p>
      </div>
    </div>
  );

  return (
    <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8 w-full">
      <header className="flex flex-col md:flex-row md:items-center md:justify-between mb-8 gap-4">
        <div>
          <h1 className="text-3xl font-bold tracking-tight text-gray-900 flex items-center gap-2">
            <span className="text-indigo-600">Flaky</span>Ollama
            <span className="bg-indigo-100 text-indigo-700 text-xs font-semibold px-2.5 py-0.5 rounded-full">v2.0 (React)</span>
          </h1>
          <p className="mt-1 text-sm text-gray-500">Autonomous cluster orchestration with Tailscale & React.</p>
        </div>
        <div className="flex items-center gap-4">
          <div className="bg-white px-4 py-2 rounded-lg shadow-sm border border-gray-200 flex items-center gap-3">
            <div className="p-2 bg-amber-50 rounded-md">
              <Layers className="w-5 h-5 text-amber-600" />
            </div>
            <div>
              <p className="text-xs text-gray-500 uppercase font-semibold text-left">Queue Depth</p>
              <p className="text-lg font-bold text-gray-900 text-left">{status.queue_depth}</p>
            </div>
          </div>
          <button onClick={fetchStatus} className="p-2 text-gray-500 hover:text-indigo-600 transition-colors">
            <RefreshCw className="w-6 h-6" />
          </button>
        </div>
      </header>

      {/* Stats Grid */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-6 mb-8 text-left">
        <div className="bg-white p-6 rounded-xl shadow-sm border border-gray-200">
          <h3 className="text-sm font-medium text-gray-500 mb-1">Total Nodes</h3>
          <div className="flex items-end gap-2">
            <span className="text-3xl font-bold text-gray-900">{Object.keys(status.nodes).length}</span>
            <span className="text-sm text-green-600 font-medium mb-1">Online</span>
          </div>
        </div>
        <div className="bg-white p-6 rounded-xl shadow-sm border border-gray-200">
          <h3 className="text-sm font-medium text-gray-500 mb-1">Active Workloads</h3>
          <div className="flex items-end gap-2">
            <span className="text-3xl font-bold text-gray-900">{status.active_workloads}</span>
            <span className="text-sm text-indigo-600 font-medium mb-1">Pending</span>
          </div>
        </div>
        <div className="bg-white p-6 rounded-xl shadow-sm border border-gray-200">
          <h3 className="text-sm font-medium text-gray-500 mb-1">Cluster Health</h3>
          <div className="flex items-center gap-2 mt-1">
            <span className="h-3 w-3 rounded-full bg-green-500 animate-pulse"></span>
            <span className="text-lg font-semibold text-gray-900">Operational</span>
          </div>
        </div>
      </div>

      {/* Node List */}
      <div className="bg-white rounded-xl shadow-sm border border-gray-200 overflow-hidden mb-8">
        <div className="px-6 py-4 border-b border-gray-100 flex justify-between items-center bg-gray-50/50">
          <h2 className="text-lg font-semibold text-gray-900">Worker Nodes</h2>
          <span className="text-xs text-gray-400 font-mono">Real-time Telemetry</span>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-left">
            <thead className="bg-gray-50/50 text-gray-500 text-xs uppercase tracking-wider">
              <tr>
                <th className="px-6 py-4 font-semibold">Node Identity</th>
                <th className="px-6 py-4 font-semibold">Health & Status</th>
                <th className="px-6 py-4 font-semibold">Hardware Utilization</th>
                <th className="px-6 py-4 font-semibold">Models</th>
                <th className="px-6 py-4 font-semibold text-right">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {Object.values(status.nodes).map((node) => (
                <tr key={node.address} className="hover:bg-gray-50/50 transition-colors">
                  <td className="px-6 py-4 align-top">
                    <div className="font-bold text-gray-900 flex items-center gap-2">
                      <Server className="w-4 h-4 text-indigo-400" />
                      {node.id}
                    </div>
                    <div className="text-sm text-gray-500 font-mono ml-6">{node.address}</div>
                  </td>
                  <td className="px-6 py-4 align-top">
                    <StateLabel state={node.state} />
                    <div className="text-xs text-gray-400 mt-1 ml-4">
                      {node.errors} errors • {new Date(node.last_seen).toLocaleTimeString()}
                    </div>
                    {node.draining && (
                      <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-amber-100 text-amber-800 mt-2">
                        Draining
                      </span>
                    )}
                  </td>
                  <td className="px-6 py-4 align-top min-w-[200px]">
                    <ProgressBar value={node.cpu_usage} label="CPU Load" />
                    <ProgressBar 
                      value={(node.vram_used / node.vram_total) * 100} 
                      label={`VRAM (${node.gpu_model})`} 
                      sublabel={`${(node.vram_used / 1e9).toFixed(1)} / ${(node.vram_total / 1e9).toFixed(1)} GB`}
                      colorClass="bg-emerald-500"
                    />
                    <div className="flex items-center gap-2 text-xs text-gray-500">
                      <Thermometer className="w-4 h-4 text-gray-400" />
                      <span className={node.gpu_temp > 80 ? 'text-red-600 font-bold' : node.gpu_temp > 70 ? 'text-amber-600' : 'text-green-600'}>
                        {node.gpu_temp.toFixed(1)}°C
                      </span>
                    </div>
                  </td>
                  <td className="px-6 py-4 align-top">
                    <div className="space-y-3">
                      <div>
                        <p className="text-[10px] font-bold text-gray-400 uppercase mb-1 tracking-wider">Loaded</p>
                        <div className="flex flex-wrap gap-1">
                          {node.active_models?.map(m => (
                            <div key={m} className="inline-flex items-center px-2 py-0.5 rounded text-[10px] font-medium bg-indigo-100 text-indigo-800 border border-indigo-200">
                              {m}
                              <button onClick={() => handleUnload(node.address, m)} className="ml-1 text-indigo-400 hover:text-red-500">
                                <XCircle className="w-3 h-3" />
                              </button>
                            </div>
                          ))}
                        </div>
                      </div>
                      <div>
                        <p className="text-[10px] font-bold text-gray-400 uppercase mb-1 tracking-wider">Local</p>
                        <div className="flex flex-wrap gap-1">
                          {node.local_models?.map(m => (
                            <div key={m.name} className="group relative inline-flex items-center px-2 py-0.5 rounded text-[10px] font-medium bg-gray-100 text-gray-800 border border-gray-200">
                              {m.name}
                              <button onClick={() => handleDelete(node.address, m.name)} className="ml-1 text-gray-400 hover:text-red-500 opacity-0 group-hover:opacity-100">
                                <Trash2 className="w-3 h-3" />
                              </button>
                            </div>
                          ))}
                        </div>
                      </div>
                    </div>
                  </td>
                  <td className="px-6 py-4 align-top text-right">
                    <div className="flex flex-col gap-2 w-32 ml-auto">
                      <button 
                        onClick={() => handleDrain(node.address, node.draining)} 
                        className={`px-3 py-1.5 font-semibold rounded-md text-xs border transition-colors ${
                          node.draining ? 'bg-white text-gray-700 border-gray-300' : 'bg-amber-50 text-amber-700 border-amber-200'
                        }`}
                      >
                        {node.draining ? 'Undrain' : 'Drain'}
                      </button>
                      <button 
                        onClick={() => handlePullNode(node.address)}
                        className="px-3 py-1.5 text-xs font-semibold rounded-md bg-indigo-50 text-indigo-700 border border-indigo-200 hover:bg-indigo-100 transition-colors"
                      >
                        Pull Model
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {/* Footer Content */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-8 text-left">
        <div className="lg:col-span-2">
          <div className="bg-white rounded-xl shadow-sm border border-gray-200 overflow-hidden">
            <div className="px-6 py-4 border-b border-gray-100 bg-gray-50/50 flex items-center gap-2">
              <Play className="w-5 h-5 text-indigo-500" />
              <h2 className="text-lg font-semibold text-gray-900">Cluster Playground</h2>
            </div>
            <div className="p-6">
              <form onSubmit={handleTest} className="space-y-4">
                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div>
                    <label className="block text-sm font-medium text-gray-700 mb-1">Target Model</label>
                    <select name="model" className="w-full rounded-md border-gray-300 shadow-sm focus:border-indigo-500 focus:ring-indigo-500 sm:text-sm">
                      {status.all_models.map(m => <option key={m} value={m}>{m}</option>)}
                    </select>
                  </div>
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700 mb-1">Test Prompt</label>
                  <textarea name="prompt" rows={3} className="w-full rounded-md border-gray-300 shadow-sm focus:border-indigo-500 focus:ring-indigo-500 sm:text-sm" placeholder="Test the cluster routing..."></textarea>
                </div>
                <button 
                  disabled={testLoading}
                  type="submit" 
                  className="inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md shadow-sm text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:opacity-50"
                >
                  {testLoading ? 'Processing...' : 'Run Inference Test'}
                </button>
              </form>

              {testResult && (
                <div className="mt-6 bg-indigo-50 border border-indigo-100 p-4 rounded-xl shadow-sm">
                  <div className="flex justify-between items-center mb-2">
                    <span className="text-xs font-bold text-indigo-700 uppercase">Response from {testResult.agent_id}</span>
                  </div>
                  <div className="text-gray-800 whitespace-pre-wrap leading-relaxed text-left">{testResult.response}</div>
                </div>
              )}
            </div>
          </div>
        </div>

        <div className="space-y-8">
          <div className="bg-white rounded-xl shadow-sm border border-gray-200 overflow-hidden">
            <div className="px-6 py-4 border-b border-gray-100 bg-gray-50/50 flex items-center gap-2">
              <RefreshCw className="w-5 h-5 text-indigo-500" />
              <h2 className="text-lg font-semibold text-gray-900">Pull Model</h2>
            </div>
            <div className="p-6">
              <form onSubmit={handlePullCluster} className="space-y-4">
                <input type="text" name="model" placeholder="e.g. llama3" className="w-full rounded-md border-gray-300 shadow-sm focus:border-indigo-500 focus:ring-indigo-500 sm:text-sm" />
                <button type="submit" className="w-full flex justify-center py-2 px-4 border border-transparent rounded-md shadow-sm text-sm font-medium text-white bg-indigo-600 hover:bg-indigo-700">
                  Pull to Cluster
                </button>
              </form>
            </div>
          </div>

          <div className="bg-white rounded-xl shadow-sm border border-gray-200 overflow-hidden">
            <div className="px-6 py-4 border-b border-gray-100 bg-gray-50/50 flex items-center gap-2">
              <Database className="w-5 h-5 text-emerald-500" />
              <h2 className="text-lg font-semibold text-gray-900">Model Catalog</h2>
            </div>
            <div className="divide-y divide-gray-100 max-h-60 overflow-y-auto">
              {status.all_models.map(m => (
                <div key={m} className="px-6 py-3 flex justify-between items-center hover:bg-gray-50/50">
                  <span className="text-sm font-medium text-gray-700">{m}</span>
                  <span className="inline-flex items-center px-2 py-0.5 rounded text-[10px] font-bold bg-emerald-100 text-emerald-700 uppercase">Online</span>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};

export default App;
