import React, { useState, useEffect } from 'react';
import { api } from './api';
import type { ClusterStatus } from './api';
import { 
  Server, Database, Thermometer, Trash2, XCircle, Play, Layers, RefreshCw, Cpu,
  Activity, AlertTriangle, CheckCircle2, CloudDownload, Terminal, ChevronDown, ChevronUp
} from 'lucide-react';
import { AnimatePresence, motion } from 'framer-motion';
import { Toaster, toast } from 'sonner';

const StateLabel = ({ state }: { state: number }) => {
  const states = ['Healthy', 'Degraded', 'Broken'];
  const colors = ['bg-emerald-500', 'bg-amber-500', 'bg-red-500'];
  const textColors = ['text-emerald-700', 'text-amber-700', 'text-red-700'];
  const icons = [
    <CheckCircle2 className="w-3 h-3 text-emerald-600" />,
    <AlertTriangle className="w-3 h-3 text-amber-600" />,
    <XCircle className="w-3 h-3 text-red-600" />
  ];
  return (
    <div className={`inline-flex items-center gap-1.5 px-2 py-1 rounded-md bg-white border shadow-sm text-xs font-medium ${textColors[state] || 'text-gray-500'}`}>
      {icons[state] || <Activity className="w-3 h-3 text-gray-400" />}
      {states[state] || 'Unknown'}
    </div>
  );
};

const ProgressBar = ({ value, label, sublabel, colorClass = "bg-indigo-500" }: { value: number, label: string, sublabel?: string, colorClass?: string }) => (
  <div className="mb-3">
    <div className="flex justify-between text-xs mb-1.5">
      <span className="text-gray-600 font-medium flex items-center gap-1">
        {label.includes('CPU') ? <Cpu className="w-3 h-3"/> : null}
        {label}
      </span>
      <span className="font-semibold text-gray-900">{sublabel || `${value.toFixed(1)}%`}</span>
    </div>
    <div className="bg-gray-100 rounded-full h-2.5 overflow-hidden border border-gray-200/50 shadow-inner">
      <motion.div
        initial={{ width: 0 }}
        animate={{ width: `${Math.min(100, Math.max(0, value))}%` }}
        transition={{ duration: 0.5, ease: "easeOut" }}
        className={`h-full ${colorClass}`}
      />
    </div>
  </div>
);

const App = () => {
  const [status, setStatus] = useState<ClusterStatus | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [testResult, setTestResult] = useState<{agent_id: string, response: string} | null>(null);
  const [testLoading, setTestLoading] = useState(false);
  const [expandedNodes, setExpandedNodes] = useState<Record<string, boolean>>({});

  const toggleNode = (id: string) => {
    setExpandedNodes(prev => ({ ...prev, [id]: !prev[id] }));
  };

  const fetchStatus = async (silent = false) => {
    try {
      const data = await api.getStatus();
      setStatus(data);
      if (error) {
        toast.success('Connection restored');
        setError(null);
      }
    } catch (err) {
      if (!error && !silent) {
        toast.error('Connection lost to balancer');
        setError('Connection lost to balancer');
      }
    }
  };

  useEffect(() => {
    fetchStatus();
    const interval = setInterval(() => fetchStatus(true), 5000);
    return () => clearInterval(interval);
  }, [error]);

  const handleAction = async (action: () => Promise<void>, successMsg: string, errorMsg: string) => {
    const toastId = toast.loading('Processing...');
    try {
      await action();
      toast.success(successMsg, { id: toastId });
      fetchStatus(true);
    } catch (err) {
      toast.error(errorMsg, { id: toastId });
    }
  };

  const handleDrain = (addr: string, draining: boolean) => {
    handleAction(
      () => draining ? api.undrainNode(addr) : api.drainNode(addr),
      `Node ${draining ? 'undrained' : 'draining'} successfully`,
      `Failed to ${draining ? 'undrain' : 'drain'} node`
    );
  };

  const handleUnload = (addr: string, model: string) => {
    handleAction(
      () => api.unloadModel(addr, model),
      `Model ${model} unloaded`,
      `Failed to unload ${model}`
    );
  };

  const handleDelete = (addr: string, model: string) => {
    if (window.confirm(`Delete ${model} from ${addr}? This action cannot be undone.`)) {
      handleAction(
        () => api.deleteModel(addr, model),
        `Model ${model} deleted`,
        `Failed to delete ${model}`
      );
    }
  };

  const handlePullNode = (addr: string) => {
    const model = window.prompt('Enter model name to pull to this node:');
    if (model) {
      handleAction(
        () => api.pullModel(model, addr),
        `Pulling ${model} to node...`,
        `Failed to pull ${model}`
      );
    }
  };

  const handlePullCluster = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const formData = new FormData(e.currentTarget);
    const model = formData.get('model') as string;
    if (model) {
      handleAction(
        () => api.pullModel(model),
        `Pulling ${model} to cluster...`,
        `Failed to pull ${model}`
      );
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
      const toastId = toast.loading('Running inference test...');
      try {
        const res = await api.runTest(model, prompt);
        setTestResult(res);
        toast.success(`Received response from ${res.agent_id}`, { id: toastId });
      } catch (err) {
        toast.error('Test failed to execute', { id: toastId });
      } finally {
        setTestLoading(false);
      }
    }
  };

  if (!status) return (
    <div className="flex items-center justify-center min-h-screen bg-slate-50">
      <div className="text-center p-8 bg-white rounded-2xl shadow-xl border border-slate-100 max-w-sm w-full">
        <motion.div
          animate={{ rotate: 360 }}
          transition={{ repeat: Infinity, duration: 1, ease: "linear" }}
          className="w-16 h-16 mx-auto mb-6 text-indigo-500 flex items-center justify-center"
        >
          <RefreshCw className="w-10 h-10" />
        </motion.div>
        <h2 className="text-xl font-bold text-slate-800 mb-2">Connecting to Cluster</h2>
        <p className="text-slate-500 font-medium">{error || 'Establishing connection with balancer...'}</p>
      </div>
    </div>
  );

  return (
    <div className="min-h-screen bg-slate-50 text-slate-900 font-sans selection:bg-indigo-100 selection:text-indigo-900 pb-12">
      <Toaster position="top-right" richColors />

      {/* Top Navigation Bar */}
      <div className="bg-white border-b border-slate-200 sticky top-0 z-30 shadow-sm">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 h-16 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="w-8 h-8 rounded-lg bg-indigo-600 flex items-center justify-center shadow-inner">
              <Server className="w-5 h-5 text-white" />
            </div>
            <div>
              <h1 className="text-xl font-bold tracking-tight text-slate-900 flex items-center gap-2">
                FlakyOllama
                <span className="bg-indigo-50 text-indigo-700 border border-indigo-200 text-[10px] uppercase font-bold px-2 py-0.5 rounded-full tracking-wider">v2.0 Dashboard</span>
              </h1>
            </div>
          </div>
          <div className="flex items-center gap-4">
            <div className="hidden sm:flex items-center gap-2 bg-slate-100 px-3 py-1.5 rounded-full text-xs font-semibold text-slate-600 border border-slate-200">
              <span className="relative flex h-2 w-2">
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
                <span className="relative inline-flex rounded-full h-2 w-2 bg-emerald-500"></span>
              </span>
              Live Sync
            </div>
            <button
              onClick={() => fetchStatus(false)}
              className="p-2 text-slate-500 hover:text-indigo-600 hover:bg-indigo-50 rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-indigo-500"
              title="Refresh Cluster Status"
            >
              <RefreshCw className="w-5 h-5" />
            </button>
          </div>
        </div>
      </div>

      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8 w-full space-y-8">

        {/* Top KPIs */}
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
          <motion.div initial={{ opacity: 0, y: 20 }} animate={{ opacity: 1, y: 0 }} className="bg-white p-5 rounded-2xl shadow-sm border border-slate-200 flex items-start gap-4">
            <div className="p-3 bg-blue-50 rounded-xl">
              <Server className="w-6 h-6 text-blue-600" />
            </div>
            <div>
              <p className="text-xs font-bold text-slate-500 uppercase tracking-wider mb-1">Total Nodes</p>
              <div className="flex items-baseline gap-2">
                <span className="text-2xl font-black text-slate-900">{Object.keys(status.nodes).length}</span>
                <span className="text-sm text-emerald-600 font-semibold">Online</span>
              </div>
            </div>
          </motion.div>

          <motion.div initial={{ opacity: 0, y: 20 }} animate={{ opacity: 1, y: 0 }} transition={{ delay: 0.1 }} className="bg-white p-5 rounded-2xl shadow-sm border border-slate-200 flex items-start gap-4">
            <div className="p-3 bg-indigo-50 rounded-xl">
              <Activity className="w-6 h-6 text-indigo-600" />
            </div>
            <div>
              <p className="text-xs font-bold text-slate-500 uppercase tracking-wider mb-1">Active Workloads</p>
              <div className="flex items-baseline gap-2">
                <span className="text-2xl font-black text-slate-900">{status.active_workloads}</span>
                <span className="text-sm text-indigo-600 font-semibold">Running</span>
              </div>
            </div>
          </motion.div>

          <motion.div initial={{ opacity: 0, y: 20 }} animate={{ opacity: 1, y: 0 }} transition={{ delay: 0.2 }} className="bg-white p-5 rounded-2xl shadow-sm border border-slate-200 flex items-start gap-4">
            <div className="p-3 bg-amber-50 rounded-xl">
              <Layers className="w-6 h-6 text-amber-600" />
            </div>
            <div>
              <p className="text-xs font-bold text-slate-500 uppercase tracking-wider mb-1">Queue Depth</p>
              <div className="flex items-baseline gap-2">
                <span className="text-2xl font-black text-slate-900">{status.queue_depth}</span>
                <span className="text-sm text-amber-600 font-semibold">Pending</span>
              </div>
            </div>
          </motion.div>

          <motion.div initial={{ opacity: 0, y: 20 }} animate={{ opacity: 1, y: 0 }} transition={{ delay: 0.3 }} className="bg-white p-5 rounded-2xl shadow-sm border border-slate-200 flex items-start gap-4">
            <div className="p-3 bg-emerald-50 rounded-xl">
              <Database className="w-6 h-6 text-emerald-600" />
            </div>
            <div>
              <p className="text-xs font-bold text-slate-500 uppercase tracking-wider mb-1">Total Models</p>
              <div className="flex items-baseline gap-2">
                <span className="text-2xl font-black text-slate-900">{status.all_models.length}</span>
                <span className="text-sm text-emerald-600 font-semibold">Available</span>
              </div>
            </div>
          </motion.div>
        </div>

        {/* Worker Nodes Grid */}
        <div>
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-lg font-bold text-slate-900 flex items-center gap-2">
              <Server className="w-5 h-5 text-indigo-600" />
              Worker Fleet
            </h2>
            <span className="text-xs font-semibold text-slate-500 bg-slate-200 px-2.5 py-1 rounded-full">{Object.keys(status.nodes).length} Nodes</span>
          </div>

          <div className="grid grid-cols-1 xl:grid-cols-2 gap-6">
            <AnimatePresence>
              {Object.values(status.nodes).map((node) => {
                const isExpanded = expandedNodes[node.id] || false;
                return (
                  <motion.div
                    layout
                    initial={{ opacity: 0, scale: 0.95 }}
                    animate={{ opacity: 1, scale: 1 }}
                    exit={{ opacity: 0, scale: 0.95 }}
                    key={node.address}
                    className={`bg-white rounded-2xl shadow-sm border transition-all duration-300 ${node.draining ? 'border-amber-300 bg-amber-50/30' : 'border-slate-200 hover:border-indigo-300 hover:shadow-md'}`}
                  >
                    {/* Node Header */}
                    <div className="p-5 border-b border-slate-100 flex flex-wrap gap-4 items-start justify-between">
                      <div className="flex items-start gap-3">
                        <div className={`p-2.5 rounded-xl ${node.draining ? 'bg-amber-100 text-amber-600' : 'bg-indigo-50 text-indigo-600'}`}>
                          <Server className="w-5 h-5" />
                        </div>
                        <div>
                          <div className="flex items-center gap-2">
                            <h3 className="font-bold text-slate-900 text-lg">{node.id}</h3>
                            <StateLabel state={node.state} />
                            {node.draining && (
                              <span className="inline-flex items-center px-2 py-0.5 rounded-md text-[10px] font-bold bg-amber-500 text-white uppercase tracking-wider animate-pulse">
                                Draining
                              </span>
                            )}
                          </div>
                          <p className="text-xs text-slate-500 font-mono mt-0.5">{node.address}</p>
                        </div>
                      </div>
                      <div className="flex flex-wrap items-center gap-2">
                        <button
                          onClick={() => handleDrain(node.address, node.draining)}
                          className={`px-3 py-1.5 font-bold rounded-lg text-xs transition-colors shadow-sm ${
                            node.draining 
                              ? 'bg-amber-100 text-amber-800 hover:bg-amber-200' 
                              : 'bg-white text-slate-700 border border-slate-200 hover:bg-slate-50 hover:text-amber-600'
                          }`}
                        >
                          {node.draining ? 'Cancel Drain' : 'Drain Node'}
                        </button>
                        <button
                          onClick={() => toggleNode(node.id)}
                          className="p-1.5 text-slate-400 hover:text-indigo-600 hover:bg-indigo-50 rounded-lg transition-colors border border-transparent hover:border-indigo-100"
                        >
                          {isExpanded ? <ChevronUp className="w-4 h-4" /> : <ChevronDown className="w-4 h-4" />}
                        </button>
                      </div>
                    </div>

                    {/* Node Stats */}
                    <div className="p-5 bg-slate-50/50">
                      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                        <div>
                          <ProgressBar value={node.cpu_usage} label="CPU Utilization" colorClass="bg-blue-500" />
                          <div className="flex items-center justify-between text-xs mt-2 text-slate-600">
                            <span className="flex items-center gap-1"><Cpu className="w-3.5 h-3.5"/>Cores: {node.cpu_cores}</span>
                            <span>Errors: <span className={node.errors > 0 ? "text-red-600 font-bold" : "text-emerald-600"}>{node.errors}</span></span>
                          </div>
                        </div>
                        <div>
                          <ProgressBar
                            value={(node.vram_used / node.vram_total) * 100}
                            label={`GPU: ${node.gpu_model}`}
                            sublabel={`${(node.vram_used / 1e9).toFixed(1)} / ${(node.vram_total / 1e9).toFixed(1)} GB VRAM`}
                            colorClass="bg-emerald-500"
                          />
                          <div className="flex items-center gap-2 text-xs font-medium mt-2">
                            <Thermometer className="w-3.5 h-3.5 text-slate-400" />
                            <span className="text-slate-600">Temp: </span>
                            <span className={node.gpu_temp > 80 ? 'text-red-600 font-bold' : node.gpu_temp > 70 ? 'text-amber-600 font-bold' : 'text-emerald-600 font-bold'}>
                              {node.gpu_temp.toFixed(1)}°C
                            </span>
                          </div>
                        </div>
                      </div>
                    </div>

                    {/* Expanded Models Section */}
                    <AnimatePresence>
                      {isExpanded && (
                        <motion.div
                          initial={{ height: 0, opacity: 0 }}
                          animate={{ height: "auto", opacity: 1 }}
                          exit={{ height: 0, opacity: 0 }}
                          className="overflow-hidden border-t border-slate-100"
                        >
                          <div className="p-5 bg-white space-y-5">
                            <div>
                              <div className="flex items-center justify-between mb-2">
                                <p className="text-xs font-bold text-slate-500 uppercase tracking-wider flex items-center gap-1.5">
                                  <Terminal className="w-3.5 h-3.5" />
                                  Active Workload Models
                                </p>
                              </div>
                              <div className="flex flex-wrap gap-2">
                                {node.active_models?.length ? node.active_models.map(m => (
                                  <div key={m} className="group flex items-center gap-1 px-2.5 py-1 rounded-lg text-xs font-semibold bg-indigo-50 text-indigo-700 border border-indigo-100/50 shadow-sm transition-all hover:shadow hover:border-indigo-200">
                                    <span className="relative flex h-1.5 w-1.5 mr-1">
                                      <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-indigo-400 opacity-75"></span>
                                      <span className="relative inline-flex rounded-full h-1.5 w-1.5 bg-indigo-500"></span>
                                    </span>
                                    {m}
                                    <button
                                      onClick={() => handleUnload(node.address, m)}
                                      className="ml-1.5 text-indigo-300 hover:text-red-500 transition-colors"
                                      title="Unload from VRAM"
                                    >
                                      <XCircle className="w-3.5 h-3.5" />
                                    </button>
                                  </div>
                                )) : (
                                  <span className="text-xs text-slate-400 italic">No models currently loaded in VRAM</span>
                                )}
                              </div>
                            </div>

                            <div>
                              <div className="flex items-center justify-between mb-2">
                                <p className="text-xs font-bold text-slate-500 uppercase tracking-wider flex items-center gap-1.5">
                                  <Database className="w-3.5 h-3.5" />
                                  Downloaded Models
                                </p>
                                <button
                                  onClick={() => handlePullNode(node.address)}
                                  className="text-xs font-bold text-indigo-600 hover:text-indigo-800 flex items-center gap-1"
                                >
                                  <CloudDownload className="w-3 h-3" /> Pull New
                                </button>
                              </div>
                              <div className="flex flex-wrap gap-2">
                                {node.local_models?.length ? node.local_models.map(m => (
                                  <div key={m.name} className="group flex items-center gap-1 px-2.5 py-1 rounded-lg text-xs font-semibold bg-slate-50 text-slate-700 border border-slate-200 shadow-sm hover:shadow hover:border-slate-300 transition-all">
                                    {m.name}
                                    <span className="text-[10px] text-slate-400 font-normal ml-1">({(m.size / 1e9).toFixed(1)}GB)</span>
                                    <button
                                      onClick={() => handleDelete(node.address, m.name)}
                                      className="ml-1.5 text-slate-300 hover:text-red-500 transition-colors opacity-0 group-hover:opacity-100 focus:opacity-100"
                                      title="Delete from disk"
                                    >
                                      <Trash2 className="w-3.5 h-3.5" />
                                    </button>
                                  </div>
                                )) : (
                                  <span className="text-xs text-slate-400 italic">No models on disk</span>
                                )}
                              </div>
                            </div>
                          </div>
                        </motion.div>
                      )}
                    </AnimatePresence>
                  </motion.div>
                );
              })}
            </AnimatePresence>
          </div>
        </div>

        {/* Lower Section: Playground and Catalog */}
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">

          {/* Playground */}
          <div className="lg:col-span-2 bg-white rounded-2xl shadow-sm border border-slate-200 overflow-hidden flex flex-col">
            <div className="px-6 py-4 border-b border-slate-100 bg-slate-50/50 flex items-center gap-3">
              <div className="p-1.5 bg-indigo-100 rounded-lg">
                <Play className="w-4 h-4 text-indigo-600" />
              </div>
              <h2 className="text-base font-bold text-slate-900">Inference Playground</h2>
            </div>
            <div className="p-6 flex-1 flex flex-col">
              <form onSubmit={handleTest} className="space-y-5">
                <div>
                  <label className="block text-xs font-bold text-slate-500 uppercase tracking-wider mb-2">Target Model</label>
                  <select
                    name="model"
                    className="w-full rounded-xl border-slate-200 shadow-sm focus:border-indigo-500 focus:ring-indigo-500 text-sm font-medium text-slate-700 bg-slate-50 py-2.5"
                    required
                  >
                    {!status.all_models.length && <option value="">No models available in cluster</option>}
                    {status.all_models.map(m => <option key={m} value={m}>{m}</option>)}
                  </select>
                </div>
                <div>
                  <label className="block text-xs font-bold text-slate-500 uppercase tracking-wider mb-2">Prompt</label>
                  <textarea
                    name="prompt"
                    rows={4}
                    className="w-full rounded-xl border-slate-200 shadow-sm focus:border-indigo-500 focus:ring-indigo-500 text-sm p-3 bg-slate-50"
                    placeholder="Write a creative prompt to test cluster routing and orchestration..."
                    required
                  ></textarea>
                </div>
                <div className="flex justify-end">
                  <button
                    disabled={testLoading || status.all_models.length === 0}
                    type="submit"
                    className="inline-flex items-center gap-2 px-6 py-2.5 border border-transparent text-sm font-bold rounded-xl shadow-sm text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed transition-all"
                  >
                    {testLoading ? (
                      <><RefreshCw className="w-4 h-4 animate-spin" /> Processing...</>
                    ) : (
                      <><Play className="w-4 h-4" /> Run Inference</>
                    )}
                  </button>
                </div>
              </form>

              <AnimatePresence>
                {testResult && (
                  <motion.div
                    initial={{ opacity: 0, y: 10 }}
                    animate={{ opacity: 1, y: 0 }}
                    className="mt-6 bg-slate-900 rounded-xl overflow-hidden shadow-inner flex-1 flex flex-col"
                  >
                    <div className="px-4 py-2 bg-slate-800 border-b border-slate-700 flex items-center justify-between">
                      <span className="text-[10px] font-mono text-slate-400">Response</span>
                      <span className="text-[10px] font-mono text-emerald-400 bg-emerald-400/10 px-2 py-0.5 rounded-full">Served by: {testResult.agent_id}</span>
                    </div>
                    <div className="p-4 text-slate-300 text-sm font-mono whitespace-pre-wrap leading-relaxed overflow-y-auto max-h-60">
                      {testResult.response}
                    </div>
                  </motion.div>
                )}
              </AnimatePresence>
            </div>
          </div>

          <div className="space-y-6 flex flex-col">
            {/* Global Pull */}
            <div className="bg-white rounded-2xl shadow-sm border border-slate-200 overflow-hidden">
              <div className="px-5 py-3.5 border-b border-slate-100 bg-slate-50/50 flex items-center gap-2.5">
                <div className="p-1.5 bg-blue-100 rounded-lg">
                  <CloudDownload className="w-4 h-4 text-blue-600" />
                </div>
                <h2 className="text-sm font-bold text-slate-900">Pull to Cluster</h2>
              </div>
              <div className="p-5">
                <form onSubmit={handlePullCluster} className="space-y-3">
                  <p className="text-xs text-slate-500 mb-2">Deploy a new model across available cluster nodes dynamically.</p>
                  <input
                    type="text"
                    name="model"
                    placeholder="e.g. llama3:8b, mistral"
                    className="w-full rounded-xl border-slate-200 shadow-sm focus:border-blue-500 focus:ring-blue-500 text-sm bg-slate-50 py-2.5"
                    required
                  />
                  <button type="submit" className="w-full flex justify-center py-2.5 px-4 border border-transparent rounded-xl shadow-sm text-sm font-bold text-white bg-blue-600 hover:bg-blue-700 transition-colors">
                    Execute Pull
                  </button>
                </form>
              </div>
            </div>

            {/* Model Catalog */}
            <div className="bg-white rounded-2xl shadow-sm border border-slate-200 overflow-hidden flex-1 flex flex-col">
              <div className="px-5 py-3.5 border-b border-slate-100 bg-slate-50/50 flex items-center gap-2.5">
                <div className="p-1.5 bg-emerald-100 rounded-lg">
                  <Database className="w-4 h-4 text-emerald-600" />
                </div>
                <h2 className="text-sm font-bold text-slate-900">Cluster Model Registry</h2>
              </div>
              <div className="divide-y divide-slate-100 overflow-y-auto max-h-64 flex-1">
                {status.all_models.length === 0 ? (
                  <div className="p-6 text-center text-sm text-slate-500 italic">No models currently available</div>
                ) : (
                  status.all_models.map(m => (
                    <div key={m} className="px-5 py-3 flex justify-between items-center hover:bg-slate-50/80 transition-colors">
                      <span className="text-sm font-bold text-slate-700">{m}</span>
                      <span className="inline-flex items-center px-2 py-0.5 rounded-md text-[10px] font-black bg-emerald-100 text-emerald-700 uppercase tracking-widest shadow-sm border border-emerald-200/50">
                        Ready
                      </span>
                    </div>
                  ))
                )}
              </div>
            </div>
          </div>

        </div>
      </div>
    </div>
  );
};

export default App;
