import React, { useState, useEffect } from 'react';
import {api, type NodeStatus} from './api';
import type { ClusterStatus } from './api';
import { 
  Server, Database, Thermometer, Trash2, XCircle, Play, Layers, RefreshCw, Cpu,
  Activity, AlertTriangle, CheckCircle2, CloudDownload, Terminal, ChevronDown, ChevronUp,
  Network, Zap
} from 'lucide-react';
import { AnimatePresence, motion } from 'framer-motion';
import { Toaster, toast } from 'sonner';

const ClusterTopology = ({ status }: { status: ClusterStatus }) => {
  const nodeEntries = Object.entries(status.nodes);
  const radius = 140;

  return (
    <div className="bg-white rounded-2xl shadow-sm border border-slate-200 overflow-hidden">
      <div className="px-5 py-3 border-b border-slate-100 bg-slate-50/50 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Network className="w-4 h-4 text-indigo-600" />
          <h3 className="text-sm font-bold text-slate-900">Routing Topology</h3>
        </div>
        <div className="flex items-center gap-4 text-[10px] font-bold uppercase tracking-wider text-slate-400">
          <div className="flex items-center gap-1.5"><div className="w-2 h-2 rounded-full bg-indigo-500"></div> Balancer</div>
          <div className="flex items-center gap-1.5"><div className="w-2 h-2 rounded-full bg-slate-200 border border-slate-300"></div> Node</div>
          <div className="flex items-center gap-1.5"><div className="w-2 h-2 rounded-full bg-amber-400"></div> Draining</div>
        </div>
      </div>
      <div className="relative h-[380px] w-full flex items-center justify-center bg-[radial-gradient(#e2e8f0_1px,transparent_1px)] [background-size:20px_20px]">
        
        {/* Central Hub (Balancer) */}
        <motion.div 
          initial={{ scale: 0 }}
          animate={{ scale: 1 }}
          className="relative z-20"
        >
          <div className="w-16 h-16 bg-indigo-600 rounded-2xl shadow-xl shadow-indigo-200 flex items-center justify-center border-4 border-white">
            <Zap className="w-8 h-8 text-white fill-white/20" />
          </div>
          <div className="absolute -bottom-8 left-1/2 -translate-x-1/2 whitespace-nowrap">
            <span className="bg-indigo-600 text-white text-[10px] font-black px-2 py-0.5 rounded-full uppercase shadow-sm">Balancer Hub</span>
          </div>
          
          {/* Animated Glow */}
          <motion.div 
            animate={{ scale: [1, 1.2, 1], opacity: [0.2, 0.4, 0.2] }}
            transition={{ duration: 3, repeat: Infinity }}
            className="absolute inset-0 bg-indigo-400 rounded-2xl blur-xl -z-10"
          />
        </motion.div>

        {/* Nodes and Connections */}
        {nodeEntries.map(([addr, node], i) => {
          const angle = (i / nodeEntries.length) * 2 * Math.PI - Math.PI / 2;
          const x = Math.cos(angle) * radius;
          const y = Math.sin(angle) * radius;
          const isActive = node.active_models.length > 0;

          return (
            <div key={addr} className="absolute inset-0 flex items-center justify-center pointer-events-none">
              
              {/* Connector Line */}
              <svg className="absolute inset-0 w-full h-full pointer-events-none overflow-visible">
                <motion.line
                  initial={{ pathLength: 0, opacity: 0 }}
                  animate={{ pathLength: 1, opacity: 1 }}
                  x1="50%" y1="50%"
                  x2={`calc(50% + ${x}px)`} y2={`calc(50% + ${y}px)`}
                  stroke={node.draining ? "#fbbf24" : isActive ? "#6366f1" : "#e2e8f0"}
                  strokeWidth={isActive ? "2" : "1"}
                  strokeDasharray={node.draining ? "4 4" : "0"}
                />
                
                {/* Active Traffic Pulse */}
                {isActive && (
                  <motion.circle
                    r="3"
                    fill="#818cf8"
                    animate={{ 
                      cx: ["50%", `calc(50% + ${x}px)`],
                      cy: ["50%", `calc(50% + ${y}px)`],
                      opacity: [0, 1, 0]
                    }}
                    transition={{ 
                      duration: 1.5 + (i * 0.2), 
                      repeat: Infinity, 
                      ease: "easeInOut" 
                    }}
                  />
                )}
              </svg>

              {/* Node Card */}
              <motion.div
                initial={{ opacity: 0, scale: 0 }}
                animate={{ opacity: 1, scale: 1 }}
                transition={{ delay: i * 0.1 }}
                style={{ 
                  transform: `translate(${x}px, ${y}px)`
                }}
                className="absolute pointer-events-auto"
              >
                <div className={`group relative p-2.5 rounded-xl border-2 shadow-sm transition-all duration-300 ${
                  node.draining ? 'bg-amber-50 border-amber-300 shadow-amber-100' : 'bg-white border-slate-200 hover:border-indigo-400 hover:shadow-lg'
                }`}>
                  {node.has_gpu ? (
                    <Zap className={`w-6 h-6 ${node.draining ? 'text-amber-500' : isActive ? 'text-indigo-600' : 'text-slate-400'}`} />
                  ) : (
                    <Cpu className={`w-6 h-6 ${node.draining ? 'text-amber-500' : isActive ? 'text-indigo-600' : 'text-slate-400'}`} />
                  )}
                  
                  {/* Tooltip */}
                  <div className="absolute bottom-full mb-3 left-1/2 -translate-x-1/2 opacity-0 group-hover:opacity-100 transition-opacity pointer-events-none z-30">
                    <div className="bg-slate-900 text-white text-[10px] p-2 rounded-lg shadow-xl border border-slate-700 whitespace-nowrap">
                      <div className="font-bold border-b border-white/10 pb-1 mb-1 flex items-center gap-2">
                        {node.id}
                        <span className="text-[8px] bg-white/20 px-1 rounded">{node.has_gpu ? 'GPU' : 'CPU'}</span>
                      </div>
                      <div className="flex flex-col gap-0.5 opacity-80">
                        <span>CPU: {node.cpu_usage.toFixed(1)}%</span>
                        {node.has_gpu ? (
                          <span>VRAM: {((node.vram_used / node.vram_total) * 100).toFixed(1)}%</span>
                        ) : (
                          <span>RAM: {node.memory_usage.toFixed(1)}%</span>
                        )}
                        <span className="text-indigo-300">Models: {node.active_models.length}</span>
                      </div>
                    </div>
                    <div className="w-2 h-2 bg-slate-900 rotate-45 mx-auto -mt-1 border-r border-b border-slate-700"></div>
                  </div>
                </div>
                
                {/* Node ID Label */}
                <div className="absolute top-full mt-2 left-1/2 -translate-x-1/2 text-[9px] font-black text-slate-500 whitespace-nowrap bg-white/90 backdrop-blur-sm border border-slate-100 px-1.5 py-0.5 rounded shadow-sm">
                  {node.id.split('-').pop()}
                </div>
              </motion.div>
            </div>
          );
        })}
      </div>
    </div>
  );
};

const StateLabel = ({ state }: { state: number }) => {
  const states = ['Healthy', 'Degraded', 'Broken'];
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
  const [logs, setLogs] = useState<string[]>([]);
  const [showLogs, setShowLogs] = useState(false);
  const [selectedNode, setSelectedNode] = useState<string | null>(null);

  const toggleNode = (id: string) => {
    setExpandedNodes(prev => ({ ...prev, [id]: !prev[id] }));
  };

  useEffect(() => {
    const cleanup = api.streamLogs((msg) => {
      setLogs(prev => [...prev.slice(-99), msg]);
    });
    return cleanup;
  }, []);

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

  const handleUnload = (model: string, addr?: string) => {
    handleAction(
      () => api.unloadModel(model, addr),
      addr ? `Model ${model} unloaded from ${addr}` : `Model ${model} unloaded globally`,
      `Failed to unload ${model}`
    );
  };

  const handleDelete = (model: string, addr?: string) => {
    if (window.confirm(addr ? `Delete ${model} from ${addr}? This action cannot be undone.` : `Delete ${model} from ALL nodes? This action cannot be undone.`)) {
      handleAction(
        () => api.deleteModel(model, addr),
        addr ? `Model ${model} deleted from ${addr}` : `Model ${model} deleted globally`,
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
    const nodeAddr = formData.get('node_addr') as string;

    if (model && prompt) {
      setTestLoading(true);
      const toastId = toast.loading(nodeAddr ? `Running targeted inference on ${nodeAddr}...` : 'Running inference test...');
      try {
        const res = nodeAddr 
          ? await api.runTestOnNode(model, prompt, nodeAddr)
          : await api.runTest(model, prompt);
        setTestResult(res);
        toast.success(`Received response from ${res.agent_id}`, { id: toastId });
      } catch (err) {
        toast.error('Test failed to execute', { id: toastId });
      } finally {
        setTestLoading(false);
      }
    }
  };

  const LogViewer = () => (
    <div className="bg-slate-900 rounded-2xl shadow-xl border border-slate-700 overflow-hidden flex flex-col h-[400px]">
      <div className="px-4 py-3 bg-slate-800 border-b border-slate-700 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Terminal className="w-4 h-4 text-indigo-400" />
          <h3 className="text-sm font-bold text-slate-200 uppercase tracking-wider">Live Cluster Logs</h3>
        </div>
        <button onClick={() => setLogs([])} className="text-[10px] font-bold text-slate-400 hover:text-white transition-colors uppercase tracking-widest bg-slate-700 px-2 py-1 rounded">Clear</button>
      </div>
      <div className="p-4 overflow-y-auto font-mono text-[11px] leading-relaxed text-indigo-300 flex-1 flex flex-col-reverse">
        <div>
          {logs.map((log, i) => (
            <div key={i} className="mb-1 opacity-90 border-l-2 border-indigo-500/30 pl-2 hover:bg-white/5 transition-colors">
              <span className="text-slate-500 mr-2">[{new Date().toLocaleTimeString()}]</span>
              {log}
            </div>
          )).reverse()}
        </div>
      </div>
    </div>
  );

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
              onClick={() => setShowLogs(!showLogs)}
              className={`p-2 rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-indigo-500 ${showLogs ? 'bg-indigo-600 text-white shadow-md' : 'text-slate-500 hover:text-indigo-600 hover:bg-indigo-50'}`}
              title="Toggle Live Logs"
            >
              <Terminal className="w-5 h-5" />
            </button>
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

        {/* Live Logs Section */}
        <AnimatePresence>
          {showLogs && (
            <motion.div
              initial={{ height: 0, opacity: 0 }}
              animate={{ height: "auto", opacity: 1 }}
              exit={{ height: 0, opacity: 0 }}
              className="overflow-hidden"
            >
              <LogViewer />
            </motion.div>
          )}
        </AnimatePresence>

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

        {/* Topology Visualization */}
        <motion.div initial={{ opacity: 0, y: 20 }} animate={{ opacity: 1, y: 0 }} transition={{ delay: 0.4 }}>
          <ClusterTopology status={status} />
        </motion.div>

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
              {Object.values(status.nodes).map((node: NodeStatus) => {
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
                        <div className={`p-2.5 rounded-xl ${node.draining ? 'bg-amber-100 text-amber-600' : node.has_gpu ? 'bg-indigo-50 text-indigo-600' : 'bg-slate-100 text-slate-600'}`}>
                          {node.has_gpu ? <Zap className="w-5 h-5" /> : <Cpu className="w-5 h-5" />}
                        </div>
                        <div>
                          <div className="flex items-center gap-2">
                            <h3 className="font-bold text-slate-900 text-lg">{node.id}</h3>
                            <span className={`text-[10px] font-black px-1.5 py-0.5 rounded uppercase tracking-tighter ${node.has_gpu ? 'bg-indigo-600 text-white' : 'bg-slate-200 text-slate-600'}`}>
                              {node.has_gpu ? 'GPU' : 'CPU'}
                            </span>
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
                          {node.has_gpu ? (
                            <>
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
                            </>
                          ) : (
                            <>
                              <ProgressBar
                                value={node.memory_usage}
                                label="System Memory"
                                colorClass="bg-emerald-500"
                              />
                              <div className="flex items-center gap-2 text-xs font-medium mt-2 text-slate-500">
                                <Layers className="w-3.5 h-3.5 opacity-50" />
                                <span>Memory-only inference mode</span>
                              </div>
                            </>
                          )}
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
                                    <div className="flex items-center gap-1 ml-1 opacity-0 group-hover:opacity-100 transition-opacity">
                                      <button
                                        onClick={() => {
                                          setSelectedNode(node.address);
                                          const select = document.querySelector('select[name="model"]') as HTMLSelectElement;
                                          if (select) select.value = m.name;
                                          window.scrollTo({ top: document.body.scrollHeight, behavior: 'smooth' });
                                        }}
                                        className="text-slate-300 hover:text-indigo-600 transition-colors"
                                        title="Run inference here"
                                      >
                                        <Play className="w-3.5 h-3.5" />
                                      </button>
                                      <button
                                        onClick={() => handleDelete(node.address, m.name)}
                                        className="text-slate-300 hover:text-red-500 transition-colors"
                                        title="Delete from disk"
                                      >
                                        <Trash2 className="w-3.5 h-3.5" />
                                      </button>
                                    </div>
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
                <div className="grid grid-cols-1 md:grid-cols-2 gap-5">
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
                    <label className="block text-xs font-bold text-slate-500 uppercase tracking-wider mb-2">Target Node (Optional)</label>
                    <div className="relative">
                      <select
                        name="node_addr"
                        value={selectedNode || ""}
                        onChange={(e) => setSelectedNode(e.target.value || null)}
                        className="w-full rounded-xl border-slate-200 shadow-sm focus:border-indigo-500 focus:ring-indigo-500 text-sm font-medium text-slate-700 bg-slate-50 py-2.5 pl-3 pr-10 appearance-none"
                      >
                        <option value="">Dynamic Routing (Best Node)</option>
                        {Object.values(status.nodes).map((n: NodeStatus) => (
                          <option key={n.address} value={n.address}>{n.id} ({n.address})</option>
                        ))}
                      </select>
                      <div className="absolute inset-y-0 right-0 flex items-center pr-3 pointer-events-none text-slate-400">
                        <ChevronDown className="w-4 h-4" />
                      </div>
                    </div>
                  </div>
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
                    <div key={m} className="px-5 py-3 flex justify-between items-center group hover:bg-slate-50/80 transition-colors">
                      <span className="text-sm font-bold text-slate-700">{m}</span>
                      <div className="flex items-center gap-3">
                        <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity mr-2">
                          <button
                            onClick={() => handleUnload(m)}
                            className="p-1.5 text-slate-400 hover:text-amber-500 bg-white rounded-lg border border-slate-200 shadow-sm transition-colors"
                            title="Unload from all VRAMs"
                          >
                            <XCircle className="w-3.5 h-3.5" />
                          </button>
                          <button
                            onClick={() => handleDelete(m)}
                            className="p-1.5 text-slate-400 hover:text-red-500 bg-white rounded-lg border border-slate-200 shadow-sm transition-colors"
                            title="Delete from all disks"
                          >
                            <Trash2 className="w-3.5 h-3.5" />
                          </button>
                        </div>
                        <span className="inline-flex items-center px-2 py-0.5 rounded-md text-[10px] font-black bg-emerald-100 text-emerald-700 uppercase tracking-widest shadow-sm border border-emerald-200/50">
                          Ready
                        </span>
                      </div>
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
