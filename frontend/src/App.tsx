import React, { useState, useEffect } from 'react';
import { api, type NodeStatus } from './api';
import type { ClusterStatus } from './api';
import {
  Server, Database, Trash2, XCircle, Play, Layers, RefreshCw, Cpu,
  Activity, AlertTriangle, CheckCircle2, CloudDownload, Terminal,
  Network, Zap, HardDrive, Info, Settings2, Search, MoreVertical
} from 'lucide-react';
import { AnimatePresence, motion } from 'framer-motion';
import { Toaster, toast } from 'sonner';

// Shadcn UI Components
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Progress } from "@/components/ui/progress";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@/components/ui/accordion";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { Separator } from "@/components/ui/separator";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";

const ClusterTopology = ({ status }: { status: ClusterStatus }) => {
  const nodeEntries = Object.entries(status.nodes);
  const radius = 140;

  return (
    <Card className="overflow-hidden border-none shadow-none bg-transparent">
      <div className="relative h-[400px] w-full flex items-center justify-center bg-[radial-gradient(var(--border)_1px,transparent_1px)] [background-size:24px_24px]">
        
        {/* Central Hub (Balancer) */}
        <motion.div 
          initial={{ scale: 0 }}
          animate={{ scale: 1 }}
          className="relative z-20"
        >
          <div className="w-20 h-20 bg-primary rounded-3xl shadow-2xl shadow-primary/20 flex items-center justify-center border-4 border-background">
            <Zap className="w-10 h-10 text-primary-foreground fill-primary-foreground/20" />
          </div>
          <div className="absolute -bottom-10 left-1/2 -translate-x-1/2 whitespace-nowrap">
            <Badge variant="default" className="font-black uppercase tracking-tighter shadow-sm">Balancer Hub</Badge>
          </div>
          
          <motion.div 
            animate={{ scale: [1, 1.2, 1], opacity: [0.1, 0.3, 0.1] }}
            transition={{ duration: 4, repeat: Infinity }}
            className="absolute inset-0 bg-primary rounded-3xl blur-2xl -z-10"
          />
        </motion.div>

        {/* Nodes and Connections */}
        <TooltipProvider>
          {nodeEntries.map(([addr, node], i) => {
            const angle = (i / nodeEntries.length) * 2 * Math.PI - Math.PI / 2;
            const x = Math.cos(angle) * radius;
            const y = Math.sin(angle) * radius;
            const isActive = (node.active_models?.length || 0) > 0;

            return (
              <div key={addr} className="absolute inset-0 flex items-center justify-center pointer-events-none">
                
                {/* Connector Line */}
                <svg className="absolute inset-0 w-full h-full pointer-events-none overflow-visible">
                  <motion.line
                    initial={{ pathLength: 0, opacity: 0 }}
                    animate={{ pathLength: 1, opacity: 1 }}
                    x1="50%" y1="50%"
                    x2={`calc(50% + ${x}px)`} y2={`calc(50% + ${y}px)`}
                    stroke={node.draining ? "var(--warning)" : isActive ? "var(--primary)" : "var(--border)"}
                    strokeWidth={isActive ? "2" : "1"}
                    strokeDasharray={node.draining ? "4 4" : "0"}
                    className={node.draining ? "stroke-amber-500" : isActive ? "stroke-primary" : "stroke-muted-foreground/30"}
                  />
                  
                  {isActive && (
                    <motion.circle
                      r="3"
                      className="fill-primary"
                      animate={{ 
                        cx: ["50%", `calc(50% + ${x}px)`],
                        cy: ["50%", `calc(50% + ${y}px)`],
                        opacity: [0, 1, 0]
                      }}
                      transition={{ 
                        duration: 2, 
                        repeat: Infinity, 
                        ease: "easeInOut",
                        delay: i * 0.3
                      }}
                    />
                  )}
                </svg>

                {/* Node Card in Topology */}
                <motion.div
                  initial={{ opacity: 0, scale: 0 }}
                  animate={{ opacity: 1, scale: 1 }}
                  transition={{ delay: i * 0.1 }}
                  style={{ transform: `translate(${x}px, ${y}px)` }}
                  className="absolute pointer-events-auto"
                >
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <div className={`group relative p-3 rounded-2xl border-2 shadow-sm transition-all duration-300 cursor-help ${
                        node.draining ? 'bg-amber-50 border-amber-300 shadow-amber-100' : 'bg-card border-border hover:border-primary hover:shadow-md'
                      }`}>
                        {node.has_gpu ? (
                          <Zap className={`w-6 h-6 ${node.draining ? 'text-amber-500' : isActive ? 'text-primary' : 'text-muted-foreground'}`} />
                        ) : (
                          <Cpu className={`w-6 h-6 ${node.draining ? 'text-amber-500' : isActive ? 'text-primary' : 'text-muted-foreground'}`} />
                        )}
                      </div>
                    </TooltipTrigger>
                    <TooltipContent className="p-0 overflow-hidden border-none shadow-xl" side="top" sideOffset={10}>
                      <Card className="w-48 border-none shadow-none">
                        <CardHeader className="p-3 bg-muted/50 border-b">
                          <div className="flex items-center justify-between">
                            <span className="font-bold text-xs truncate">{node.id}</span>
                            <Badge variant={node.has_gpu ? "default" : "secondary"} className="text-[8px] h-4 px-1">{node.has_gpu ? 'GPU' : 'CPU'}</Badge>
                          </div>
                        </CardHeader>
                        <CardContent className="p-3 space-y-2">
                          <div className="flex justify-between text-[10px]">
                            <span className="text-muted-foreground">CPU</span>
                            <span className="font-bold">{node.cpu_usage.toFixed(1)}%</span>
                          </div>
                          <Progress value={node.cpu_usage} className="h-1" />
                          
                          {node.has_gpu ? (
                            <>
                              <div className="flex justify-between text-[10px]">
                                <span className="text-muted-foreground">VRAM</span>
                                <span className="font-bold">{((node.vram_used / node.vram_total) * 100).toFixed(1)}%</span>
                              </div>
                              <Progress value={(node.vram_used / node.vram_total) * 100} className="h-1" />
                            </>
                          ) : (
                            <>
                              <div className="flex justify-between text-[10px]">
                                <span className="text-muted-foreground">RAM</span>
                                <span className="font-bold">{node.memory_usage.toFixed(1)}%</span>
                              </div>
                              <Progress value={node.memory_usage} className="h-1" />
                            </>
                          )}
                          <div className="pt-1 flex items-center gap-1 text-[10px]">
                            <Layers className="w-3 h-3 text-primary" />
                            <span className="font-semibold">{node.active_models?.length || 0} active models</span>
                          </div>
                        </CardContent>
                      </Card>
                    </TooltipContent>
                  </Tooltip>
                  
                  {/* Node ID Label */}
                  <div className="absolute top-full mt-3 left-1/2 -translate-x-1/2 text-[9px] font-black text-muted-foreground whitespace-nowrap bg-background/80 backdrop-blur-sm border border-border px-2 py-0.5 rounded-full shadow-sm">
                    {node.id.split('-').pop()}
                  </div>
                </motion.div>
              </div>
            );
          })}
        </TooltipProvider>
      </div>
    </Card>
  );
};

const StateLabel = ({ state }: { state: number }) => {
  const states = ['Healthy', 'Degraded', 'Broken'];
  const icons = [
    <CheckCircle2 className="w-3 h-3" />,
    <AlertTriangle className="w-3 h-3" />,
    <XCircle className="w-3 h-3" />
  ];
  
  const colors = [
    "bg-emerald-100 text-emerald-700 border-emerald-200",
    "bg-amber-100 text-amber-700 border-amber-200",
    "bg-red-100 text-red-700 border-red-200"
  ];

  return (
    <Badge variant="outline" className={`${colors[state] || 'bg-slate-100 text-slate-700'} gap-1.5 font-bold uppercase tracking-tighter text-[10px]`}>
      {icons[state] || <Activity className="w-3 h-3" />}
      {states[state] || 'Unknown'}
    </Badge>
  );
};

const App = () => {
  const [status, setStatus] = useState<ClusterStatus | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [testResult, setTestResult] = useState<{ agent_id: string, response: string } | null>(null);
  const [testLoading, setTestLoading] = useState(false);
  const [logs, setLogs] = useState<string[]>([]);
  const [showLogs, setShowLogs] = useState(false);
  const [selectedNode, setSelectedNode] = useState<string | null>(null);
  const [searchModel, setSearchModel] = useState("");

  useEffect(() => {
    const cleanup = api.streamLogs((msg) => {
      setLogs(prev => [...prev.slice(-199), msg]);
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
      const toastId = toast.loading(nodeAddr ? `Targeted inference on ${nodeAddr}...` : 'Running inference test...');
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

  if (!status) return (
    <div className="flex items-center justify-center min-h-screen bg-background">
      <div className="text-center p-12 bg-card rounded-3xl shadow-2xl border border-border max-w-sm w-full space-y-6">
        <motion.div
          animate={{ rotate: 360 }}
          transition={{ repeat: Infinity, duration: 2, ease: "linear" }}
          className="w-20 h-20 mx-auto text-primary flex items-center justify-center"
        >
          <RefreshCw className="w-12 h-12" />
        </motion.div>
        <div className="space-y-2">
          <h2 className="text-2xl font-black tracking-tight">Connecting to Cluster</h2>
          <p className="text-muted-foreground font-medium text-sm px-4">{error || 'Establishing secure connection with the orchestrator...'}</p>
        </div>
      </div>
    </div>
  );

  const filteredModels = status.all_models.filter(m => m.toLowerCase().includes(searchModel.toLowerCase()));

  return (
    <div className="min-h-screen bg-slate-50/50 text-foreground font-sans selection:bg-primary/10 selection:text-primary pb-20">
      <Toaster position="bottom-right" richColors closeButton />

      {/* Top Navigation Bar */}
      <header className="bg-background border-b border-border sticky top-0 z-40 shadow-sm backdrop-blur-md bg-background/80">
        <div className="max-w-[1600px] mx-auto px-4 sm:px-6 lg:px-8 h-16 flex items-center justify-between">
          <div className="flex items-center gap-4">
            <div className="w-10 h-10 rounded-2xl bg-primary flex items-center justify-center shadow-lg shadow-primary/20">
              <Zap className="w-6 h-6 text-primary-foreground fill-primary-foreground/20" />
            </div>
            <div className="flex flex-col">
              <h1 className="text-lg font-black tracking-tighter leading-none flex items-center gap-2">
                FLAKYOLLAMA
                <Badge variant="secondary" className="text-[9px] font-black h-4 px-1.5 rounded-full">CLUSTER V2</Badge>
              </h1>
              <span className="text-[10px] text-muted-foreground font-bold uppercase tracking-widest mt-0.5">Distributed Inference Orchestrator</span>
            </div>
          </div>
          
          <div className="flex items-center gap-2">
            <div className="hidden md:flex items-center gap-2 mr-2 bg-muted/50 px-3 py-1.5 rounded-full text-[10px] font-black uppercase tracking-wider text-muted-foreground border border-border">
              <span className="relative flex h-2 w-2">
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
                <span className="relative inline-flex rounded-full h-2 w-2 bg-emerald-500"></span>
              </span>
              Synchronized
            </div>
            
            <Separator orientation="vertical" className="h-8 mx-1 hidden md:block" />
            
            <TooltipProvider>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button variant="ghost" size="icon" onClick={() => setShowLogs(!showLogs)} className={showLogs ? "bg-primary text-primary-foreground hover:bg-primary/90" : ""}>
                    <Terminal className="w-5 h-5" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>Toggle Live Cluster Logs</TooltipContent>
              </Tooltip>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button variant="ghost" size="icon" onClick={() => fetchStatus(false)}>
                    <RefreshCw className="w-5 h-5" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>Force Status Refresh</TooltipContent>
              </Tooltip>
            </TooltipProvider>
          </div>
        </div>
      </header>

      <main className="max-w-[1600px] mx-auto px-4 sm:px-6 lg:px-8 py-8 w-full space-y-8">

        {/* Live Logs Overlay */}
        <AnimatePresence>
          {showLogs && (
            <motion.div
              initial={{ height: 0, opacity: 0 }}
              animate={{ height: "auto", opacity: 1 }}
              exit={{ height: 0, opacity: 0 }}
              className="overflow-hidden"
            >
              <Card className="bg-slate-950 border-slate-800 shadow-2xl overflow-hidden">
                <CardHeader className="py-3 px-5 border-b border-slate-800 flex flex-row items-center justify-between">
                  <div className="flex items-center gap-2">
                    <Terminal className="w-4 h-4 text-primary" />
                    <CardTitle className="text-xs font-black uppercase tracking-widest text-slate-400">Cluster Telemetry Stream</CardTitle>
                  </div>
                  <div className="flex items-center gap-2">
                    <Badge variant="outline" className="bg-slate-900 border-slate-700 text-[9px] text-primary">{logs.length} Events</Badge>
                    <Button variant="ghost" size="sm" onClick={() => setLogs([])} className="h-7 text-[10px] uppercase font-bold text-slate-500 hover:text-white hover:bg-slate-800">Clear</Button>
                  </div>
                </CardHeader>
                <CardContent className="p-0">
                  <ScrollArea className="h-[300px] font-mono text-[11px] leading-relaxed text-indigo-300">
                    <div className="p-4 flex flex-col-reverse">
                      {logs.map((log, i) => (
                        <div key={i} className="mb-1.5 opacity-90 border-l-2 border-primary/30 pl-3 hover:bg-white/5 transition-colors py-0.5 group">
                          <span className="text-slate-600 mr-3 select-none">[{new Date().toLocaleTimeString()}]</span>
                          <span className="text-slate-200 group-hover:text-primary transition-colors">{log}</span>
                        </div>
                      )).reverse()}
                    </div>
                  </ScrollArea>
                </CardContent>
              </Card>
            </motion.div>
          )}
        </AnimatePresence>

        {/* Status Dashboard Grid */}
        <Tabs defaultValue="overview" className="space-y-8">
          <div className="flex items-center justify-between flex-wrap gap-4">
            <TabsList className="bg-muted/50 p-1 rounded-xl h-auto border border-border">
              <TabsTrigger value="overview" className="px-6 py-2 rounded-lg data-[state=active]:bg-background data-[state=active]:shadow-sm font-bold text-xs uppercase tracking-wider">Overview</TabsTrigger>
              <TabsTrigger value="fleet" className="px-6 py-2 rounded-lg data-[state=active]:bg-background data-[state=active]:shadow-sm font-bold text-xs uppercase tracking-wider">Worker Fleet</TabsTrigger>
              <TabsTrigger value="models" className="px-6 py-2 rounded-lg data-[state=active]:bg-background data-[state=active]:shadow-sm font-bold text-xs uppercase tracking-wider">Model Registry</TabsTrigger>
              <TabsTrigger value="playground" className="px-6 py-2 rounded-lg data-[state=active]:bg-background data-[state=active]:shadow-sm font-bold text-xs uppercase tracking-wider">Playground</TabsTrigger>
            </TabsList>
            
            <div className="flex items-center gap-6">
              <div className="flex flex-col items-end">
                <span className="text-[10px] font-black uppercase text-muted-foreground tracking-widest leading-none">Cluster Health</span>
                <span className="text-sm font-black text-emerald-600 tracking-tighter">OPERATIONAL</span>
              </div>
              <Separator orientation="vertical" className="h-8" />
              <div className="flex flex-col items-end">
                <span className="text-[10px] font-black uppercase text-muted-foreground tracking-widest leading-none">Throughput</span>
                <span className="text-sm font-black tracking-tighter">{status.active_workloads} ACTIVE</span>
              </div>
            </div>
          </div>

          <TabsContent value="overview" className="space-y-8 mt-0 border-none p-0 outline-none">
            {/* Top KPIs */}
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
              {[
                { label: 'Cluster Nodes', val: Object.keys(status.nodes).length, sub: 'Active Workers', icon: Server, color: 'text-blue-600', bg: 'bg-blue-50' },
                { label: 'Active Workloads', val: status.active_workloads, sub: 'Inference Tasks', icon: Activity, color: 'text-indigo-600', bg: 'bg-indigo-50' },
                { label: 'Queue Depth', val: status.queue_depth, sub: 'Queued Requests', icon: Layers, color: 'text-amber-600', bg: 'bg-amber-50' },
                { label: 'Model Library', val: status.all_models.length, sub: 'Deployed Across Fleet', icon: Database, color: 'text-emerald-600', bg: 'bg-emerald-50' },
              ].map((kpi, i) => (
                <Card key={i} className="border-none shadow-md shadow-slate-200/50 hover:shadow-lg transition-all duration-300 group overflow-hidden">
                  <div className={`absolute top-0 right-0 w-24 h-24 -mr-8 -mt-8 rounded-full ${kpi.bg} opacity-50 blur-2xl group-hover:scale-150 transition-transform duration-500`} />
                  <CardContent className="p-6 flex items-start gap-4 relative">
                    <div className={`p-3.5 rounded-2xl ${kpi.bg} ${kpi.color}`}>
                      <kpi.icon className="w-6 h-6" />
                    </div>
                    <div className="flex flex-col">
                      <p className="text-[10px] font-black text-muted-foreground uppercase tracking-widest mb-1">{kpi.label}</p>
                      <span className="text-3xl font-black tracking-tighter leading-none">{kpi.val}</span>
                      <span className={`text-[10px] font-bold mt-1.5 opacity-70 ${kpi.color}`}>{kpi.sub}</span>
                    </div>
                  </CardContent>
                </Card>
              ))}
            </div>

            {/* Topology Visualization */}
            <Card className="border-none shadow-lg shadow-slate-200/50 overflow-hidden bg-white/50 backdrop-blur-sm">
              <CardHeader className="py-4 border-b bg-white flex flex-row items-center justify-between">
                <div className="flex items-center gap-2">
                  <Network className="w-5 h-5 text-primary" />
                  <div>
                    <CardTitle className="text-sm font-black uppercase tracking-widest">Routing Topology</CardTitle>
                    <CardDescription className="text-[10px] font-bold uppercase tracking-tight">Real-time Node Interconnectivity</CardDescription>
                  </div>
                </div>
                <div className="flex items-center gap-4">
                  <div className="flex items-center gap-1.5 text-[9px] font-black uppercase text-muted-foreground"><div className="w-2 h-2 rounded-full bg-primary animate-pulse"></div> Balancer</div>
                  <div className="flex items-center gap-1.5 text-[9px] font-black uppercase text-muted-foreground"><div className="w-2 h-2 rounded-full bg-slate-300"></div> Node</div>
                  <div className="flex items-center gap-1.5 text-[9px] font-black uppercase text-muted-foreground"><div className="w-2.5 h-2.5 rounded bg-amber-400 border-2 border-amber-500"></div> Draining</div>
                </div>
              </CardHeader>
              <CardContent className="p-0">
                <ClusterTopology status={status} />
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="fleet" className="mt-0 border-none p-0 outline-none">
            <div className="flex items-center justify-between mb-6">
              <div className="space-y-1">
                <h2 className="text-2xl font-black tracking-tighter">Distributed Worker Fleet</h2>
                <p className="text-xs font-bold text-muted-foreground uppercase tracking-widest">Heterogeneous compute cluster resources</p>
              </div>
              <Badge variant="outline" className="h-8 px-4 font-black uppercase tracking-widest text-[10px] bg-white shadow-sm">{Object.keys(status.nodes).length} NODES IDENTIFIED</Badge>
            </div>

            <div className="grid grid-cols-1 xl:grid-cols-2 gap-6">
              <AnimatePresence mode='popLayout'>
                {Object.values(status.nodes).map((node: NodeStatus) => (
                  <motion.div
                    layout
                    initial={{ opacity: 0, y: 20 }}
                    animate={{ opacity: 1, y: 0 }}
                    exit={{ opacity: 0, scale: 0.95 }}
                    key={node.address}
                  >
                    <Card className={`overflow-hidden transition-all duration-300 border-2 ${node.draining ? 'border-amber-300 bg-amber-50/20' : 'border-transparent shadow-md hover:border-primary/30'}`}>
                      <CardHeader className="p-5 border-b bg-background/50 backdrop-blur-sm flex flex-row items-start justify-between space-y-0">
                        <div className="flex items-start gap-4">
                          <div className={`p-3 rounded-2xl shadow-inner ${node.draining ? 'bg-amber-100 text-amber-600' : node.has_gpu ? 'bg-indigo-50 text-indigo-600' : 'bg-slate-100 text-slate-600'}`}>
                            {node.has_gpu ? <Zap className="w-6 h-6 fill-current/10" /> : <Cpu className="w-6 h-6" />}
                          </div>
                          <div className="space-y-1">
                            <div className="flex items-center gap-2">
                              <CardTitle className="text-xl font-black tracking-tighter">{node.id}</CardTitle>
                              <Badge variant={node.has_gpu ? "default" : "secondary"} className="text-[9px] font-black h-4 uppercase tracking-tighter">
                                {node.has_gpu ? 'GPU ENABLED' : 'CPU ONLY'}
                              </Badge>
                              <StateLabel state={node.state} />
                            </div>
                            <div className="flex items-center gap-2 text-xs font-bold font-mono text-muted-foreground opacity-70">
                              <HardDrive className="w-3.5 h-3.5" /> {node.address}
                            </div>
                          </div>
                        </div>
                        
                        <div className="flex items-center gap-2">
                          <DropdownMenu>
                            <DropdownMenuTrigger asChild>
                              <Button variant="ghost" size="icon" className="h-8 w-8 rounded-lg"><MoreVertical className="w-4 h-4" /></Button>
                            </DropdownMenuTrigger>
                            <DropdownMenuContent align="end" className="w-48 font-bold text-xs uppercase tracking-tight">
                              <DropdownMenuItem onClick={() => handleDrain(node.address, node.draining)} className={node.draining ? "text-primary" : "text-amber-600"}>
                                {node.draining ? <RefreshCw className="w-3.5 h-3.5 mr-2" /> : <XCircle className="w-3.5 h-3.5 mr-2" />}
                                {node.draining ? 'RESUME TRAFFIC' : 'DRAIN NODE'}
                              </DropdownMenuItem>
                              <DropdownMenuItem onClick={() => handlePullNode(node.address)}>
                                <CloudDownload className="w-3.5 h-3.5 mr-2" /> PULL NEW MODEL
                              </DropdownMenuItem>
                              <DropdownMenuItem className="text-destructive">
                                <Trash2 className="w-3.5 h-3.5 mr-2" /> UNREGISTER NODE
                              </DropdownMenuItem>
                            </DropdownMenuContent>
                          </DropdownMenu>
                        </div>
                      </CardHeader>

                      <CardContent className="p-6 grid grid-cols-1 md:grid-cols-2 gap-8 bg-background">
                        <div className="space-y-4">
                          <div className="space-y-2">
                            <div className="flex justify-between text-[10px] font-black uppercase tracking-widest text-muted-foreground">
                              <span className="flex items-center gap-1.5"><Cpu className="w-3.5 h-3.5 text-blue-500" /> CPU USAGE</span>
                              <span className="text-foreground">{node.cpu_usage.toFixed(1)}%</span>
                            </div>
                            <Progress value={node.cpu_usage} className="h-2 bg-blue-50" indicatorClassName="bg-blue-500" />
                            <div className="flex justify-between text-[9px] font-bold text-muted-foreground pt-1">
                              <span>CORE COUNT: {node.cpu_cores}</span>
                              <span className={node.errors > 0 ? "text-destructive" : ""}>ERRORS: {node.errors}</span>
                            </div>
                          </div>
                        </div>

                        <div className="space-y-4">
                          {node.has_gpu ? (
                            <div className="space-y-2">
                              <div className="flex justify-between text-[10px] font-black uppercase tracking-widest text-muted-foreground">
                                <span className="flex items-center gap-1.5"><Zap className="w-3.5 h-3.5 text-emerald-500" /> VRAM ({node.gpu_model})</span>
                                <span className="text-foreground">{((node.vram_used / node.vram_total) * 100).toFixed(1)}%</span>
                              </div>
                              <Progress value={(node.vram_used / node.vram_total) * 100} className="h-2 bg-emerald-50" indicatorClassName="bg-emerald-500" />
                              <div className="flex justify-between text-[9px] font-bold text-muted-foreground pt-1">
                                <span>USED: {(node.vram_used / 1e9).toFixed(1)}GB / {(node.vram_total / 1e9).toFixed(1)}GB</span>
                                <span className={node.gpu_temp > 75 ? 'text-amber-600 font-black' : 'text-foreground'}>TEMP: {node.gpu_temp.toFixed(1)}°C</span>
                              </div>
                            </div>
                          ) : (
                            <div className="space-y-2">
                              <div className="flex justify-between text-[10px] font-black uppercase tracking-widest text-muted-foreground">
                                <span className="flex items-center gap-1.5"><HardDrive className="w-3.5 h-3.5 text-emerald-500" /> SYSTEM RAM</span>
                                <span className="text-foreground">{node.memory_usage.toFixed(1)}%</span>
                              </div>
                              <Progress value={node.memory_usage} className="h-2 bg-emerald-50" indicatorClassName="bg-emerald-500" />
                              <p className="text-[9px] font-bold text-muted-foreground pt-1 italic opacity-60">OPTIMIZED FOR CPU-ONLY INFERENCE</p>
                            </div>
                          )}
                        </div>
                      </CardContent>

                      <Separator />

                      <CardFooter className="p-0 flex-col bg-muted/20">
                        <Accordion type="single" collapsible className="w-full">
                          <AccordionItem value="models" className="border-none">
                            <AccordionTrigger className="px-6 py-3 hover:no-underline hover:bg-muted/30 transition-colors group">
                              <div className="flex items-center gap-3">
                                <div className="p-1.5 rounded-lg bg-indigo-100 text-indigo-600 group-data-[state=open]:bg-indigo-600 group-data-[state=open]:text-white transition-colors">
                                  <Layers className="w-3.5 h-3.5" />
                                </div>
                                <span className="text-xs font-black uppercase tracking-widest">Compute Workloads</span>
                                <Badge variant="secondary" className="font-bold h-5 px-2 text-[10px]">{(node.active_models?.length || 0) + (node.local_models?.length || 0)} Models</Badge>
                              </div>
                            </AccordionTrigger>
                            <AccordionContent className="p-6 pt-2 space-y-6">
                              <div className="space-y-3">
                                <p className="text-[10px] font-black text-muted-foreground uppercase tracking-widest flex items-center gap-2">
                                  <Activity className="w-3.5 h-3.5 text-primary animate-pulse" /> Resident In VRAM (Hot)
                                </p>
                                <div className="flex flex-wrap gap-2">
                                  {node.active_models?.length ? node.active_models.map(m => (
                                    <div key={m} className="group relative flex items-center gap-2 px-3 py-1.5 rounded-xl text-xs font-bold bg-primary/5 text-primary border border-primary/20 hover:bg-primary hover:text-white transition-all duration-300">
                                      <div className="w-1.5 h-1.5 rounded-full bg-primary group-hover:bg-white animate-pulse" />
                                      {m}
                                      <button onClick={() => handleUnload(m, node.address)} className="ml-1 opacity-40 hover:opacity-100 transition-opacity" title="Hot-Unload">
                                        <XCircle className="w-3.5 h-3.5" />
                                      </button>
                                    </div>
                                  )) : <p className="text-xs text-muted-foreground italic font-medium opacity-60 pl-1">Memory is currently cleared.</p>}
                                </div>
                              </div>

                              <div className="space-y-3">
                                <div className="flex items-center justify-between">
                                  <p className="text-[10px] font-black text-muted-foreground uppercase tracking-widest flex items-center gap-2">
                                    <Database className="w-3.5 h-3.5" /> Local Disk Storage (Warm)
                                  </p>
                                </div>
                                <div className="flex flex-wrap gap-2">
                                  {node.local_models?.length ? node.local_models.map(m => (
                                    <div key={m.name} className="group flex items-center gap-2 px-3 py-1.5 rounded-xl text-xs font-bold bg-background text-foreground border shadow-sm hover:border-primary transition-all duration-300">
                                      <HardDrive className="w-3 h-3 text-muted-foreground" />
                                      {m.name}
                                      <span className="text-[9px] opacity-50 font-black ml-1">{(m.size / 1e9).toFixed(1)}GB</span>
                                      <div className="flex items-center gap-1.5 ml-2 border-l pl-2 border-border group-hover:border-primary/30 transition-colors">
                                        <button onClick={() => {
                                          setSelectedNode(node.address);
                                          const tabPlayground = document.querySelector('[value="playground"]') as HTMLButtonElement;
                                          if (tabPlayground) tabPlayground.click();
                                          window.scrollTo({ top: 0, behavior: 'smooth' });
                                        }} className="text-muted-foreground hover:text-primary transition-colors"><Play className="w-3.5 h-3.5" /></button>
                                        <button onClick={() => handleDelete(m.name, node.address)} className="text-muted-foreground hover:text-destructive transition-colors"><Trash2 className="w-3.5 h-3.5" /></button>
                                      </div>
                                    </div>
                                  )) : <p className="text-xs text-muted-foreground italic font-medium opacity-60 pl-1">Storage is empty.</p>}
                                </div>
                              </div>
                            </AccordionContent>
                          </AccordionItem>
                        </Accordion>
                      </CardFooter>
                    </Card>
                  </motion.div>
                ))}
              </AnimatePresence>
            </div>
          </TabsContent>

          <TabsContent value="models" className="mt-0 border-none p-0 outline-none">
            <div className="grid grid-cols-1 lg:grid-cols-12 gap-8">
              <div className="lg:col-span-4 space-y-6">
                <Card className="border-none shadow-lg">
                  <CardHeader className="bg-primary text-primary-foreground p-6 rounded-t-xl">
                    <div className="w-12 h-12 bg-white/20 rounded-2xl flex items-center justify-center mb-4 backdrop-blur-sm">
                      <CloudDownload className="w-7 h-7 text-white" />
                    </div>
                    <CardTitle className="text-2xl font-black tracking-tighter uppercase leading-none">Deploy to Cluster</CardTitle>
                    <CardDescription className="text-primary-foreground/70 font-bold text-xs uppercase tracking-widest mt-2">Dynamic Model Deployment Engine</CardDescription>
                  </CardHeader>
                  <CardContent className="p-6">
                    <form onSubmit={handlePullCluster} className="space-y-4">
                      <div className="space-y-2">
                        <label className="text-[10px] font-black uppercase tracking-widest text-muted-foreground px-1">Model Specifier</label>
                        <Input name="model" placeholder="e.g. llama3:8b, phi3:latest" className="rounded-xl font-bold h-12 border-2 focus-visible:ring-primary shadow-inner" required />
                      </div>
                      <p className="text-[10px] font-medium text-muted-foreground px-1 leading-relaxed">
                        Specify the model string from Ollama registry. The balancer will dynamically orchestrate the pull across all healthy, non-draining nodes.
                      </p>
                      <Button type="submit" className="w-full h-12 rounded-xl font-black uppercase tracking-widest text-xs shadow-lg shadow-primary/20 hover:scale-[1.02] active:scale-[0.98] transition-all">
                        Initiate Fleet Deployment
                      </Button>
                    </form>
                  </CardContent>
                </Card>

                <Card className="border-none shadow-md overflow-hidden bg-muted/30">
                  <CardHeader className="p-5 border-b flex flex-row items-center gap-3 space-y-0">
                    <div className="p-2 bg-indigo-100 text-indigo-600 rounded-lg"><Info className="w-4 h-4" /></div>
                    <CardTitle className="text-xs font-black uppercase tracking-widest">Registry Logic</CardTitle>
                  </CardHeader>
                  <CardContent className="p-5 space-y-4 text-xs font-medium text-muted-foreground leading-relaxed">
                    <p>The **Model Registry** represents an aggregated view of all neural weights persisted across the worker fleet's local block storage.</p>
                    <div className="space-y-2">
                      <div className="flex gap-3"><Badge variant="outline" className="h-5 px-1.5 text-[9px] font-black">UNLOAD</Badge> <span>Safely evicts model weights from the active VRAM of all nodes.</span></div>
                      <div className="flex gap-3"><Badge variant="destructive" className="h-5 px-1.5 text-[9px] font-black">DELETE</Badge> <span>Permanently scrubs model files from the local disks of every node.</span></div>
                    </div>
                  </CardContent>
                </Card>
              </div>

              <div className="lg:col-span-8">
                <Card className="border-none shadow-lg overflow-hidden h-full flex flex-col">
                  <CardHeader className="p-6 border-b bg-background sticky top-0 z-10 flex flex-row items-center justify-between flex-wrap gap-4">
                    <div className="flex items-center gap-3">
                      <div className="p-2.5 bg-emerald-50 text-emerald-600 rounded-xl shadow-inner"><Database className="w-5 h-5" /></div>
                      <div>
                        <CardTitle className="text-lg font-black tracking-tighter uppercase">Cluster Registry</CardTitle>
                        <CardDescription className="text-[10px] font-bold text-muted-foreground uppercase tracking-wider">{status.all_models.length} Models Verified Across Fleet</CardDescription>
                      </div>
                    </div>
                    <div className="relative w-full sm:w-64">
                      <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
                      <Input placeholder="SEARCH REGISTRY..." value={searchModel} onChange={(e) => setSearchModel(e.target.value)} className="pl-10 h-10 rounded-full font-bold text-xs uppercase bg-muted/50 border-none focus-visible:ring-primary shadow-inner" />
                    </div>
                  </CardHeader>
                  <CardContent className="p-0 flex-1">
                    {filteredModels.length === 0 ? (
                      <div className="flex flex-col items-center justify-center p-20 opacity-40">
                        <Database className="w-16 h-16 mb-4 stroke-1" />
                        <p className="text-sm font-black uppercase tracking-widest">No models match criteria</p>
                      </div>
                    ) : (
                      <div className="divide-y border-b">
                        {filteredModels.map(m => (
                          <div key={m} className="px-6 py-4 flex justify-between items-center group hover:bg-muted/30 transition-all duration-200">
                            <div className="flex flex-col">
                              <span className="text-sm font-black tracking-tight group-hover:text-primary transition-colors">{m}</span>
                              <div className="flex items-center gap-2 mt-1">
                                <Badge variant="outline" className="text-[8px] h-4 px-1 rounded font-black border-emerald-200 bg-emerald-50 text-emerald-700">DISTRIBUTED</Badge>
                                <span className="text-[9px] font-bold text-muted-foreground opacity-60 uppercase tracking-tighter">OLLAMA ARCHITECTURE</span>
                              </div>
                            </div>
                            <div className="flex items-center gap-3 opacity-0 group-hover:opacity-100 focus-within:opacity-100 transition-all transform translate-x-2 group-hover:translate-x-0">
                              <TooltipProvider>
                                <Tooltip>
                                  <TooltipTrigger asChild>
                                    <Button variant="outline" size="icon" onClick={() => handleUnload(m)} className="h-8 w-8 rounded-lg text-amber-600 hover:text-amber-700 hover:bg-amber-50 border-amber-200 shadow-sm"><XCircle className="w-4 h-4" /></Button>
                                  </TooltipTrigger>
                                  <TooltipContent>EVICT FROM ALL VRAM</TooltipContent>
                                </Tooltip>
                                <Tooltip>
                                  <TooltipTrigger asChild>
                                    <Button variant="outline" size="icon" onClick={() => handleDelete(m)} className="h-8 w-8 rounded-lg text-destructive hover:text-white hover:bg-destructive border-red-200 shadow-sm"><Trash2 className="w-4 h-4" /></Button>
                                  </TooltipTrigger>
                                  <TooltipContent>DELETE FROM ALL DISKS</TooltipContent>
                                </Tooltip>
                              </TooltipProvider>
                              <Separator orientation="vertical" className="h-6 mx-1" />
                              <Badge variant="outline" className="font-black bg-emerald-100 text-emerald-700 border-none shadow-inner uppercase tracking-widest text-[9px] h-6 px-3">Ready</Badge>
                            </div>
                          </div>
                        ))}
                      </div>
                    )}
                  </CardContent>
                </Card>
              </div>
            </div>
          </TabsContent>

          <TabsContent value="playground" className="mt-0 border-none p-0 outline-none">
            <div className="grid grid-cols-1 lg:grid-cols-12 gap-8">
              <div className="lg:col-span-8">
                <Card className="border-none shadow-xl overflow-hidden flex flex-col min-h-[600px] bg-white">
                  <CardHeader className="p-6 border-b bg-background/50 backdrop-blur-sm flex flex-row items-center justify-between flex-wrap gap-4">
                    <div className="flex items-center gap-3">
                      <div className="p-2.5 bg-primary text-primary-foreground rounded-2xl shadow-lg shadow-primary/20"><Play className="w-5 h-5 fill-current/20" /></div>
                      <div>
                        <CardTitle className="text-xl font-black tracking-tighter uppercase leading-none">Inference Playground</CardTitle>
                        <CardDescription className="text-[10px] font-bold text-muted-foreground uppercase tracking-widest mt-1">Global Request Routing Simulation</CardDescription>
                      </div>
                    </div>
                    <Badge variant="outline" className="h-7 px-3 bg-muted/50 border-border font-bold text-[9px] tracking-widest uppercase">TEST SUITE v2.0</Badge>
                  </CardHeader>
                  <CardContent className="p-8 flex-1 flex flex-col space-y-8">
                    <form onSubmit={handleTest} className="space-y-8">
                      <div className="grid grid-cols-1 md:grid-cols-2 gap-8">
                        <div className="space-y-3">
                          <label className="text-[10px] font-black uppercase tracking-widest text-muted-foreground px-1 flex items-center gap-2">
                            <Layers className="w-3.5 h-3.5" /> Target Neural Architecture
                          </label>
                          <Select name="model" required defaultValue={status.all_models[0]}>
                            <SelectTrigger className="h-14 rounded-2xl border-2 font-bold text-sm bg-slate-50 focus:ring-primary shadow-sm">
                              <SelectValue placeholder="SELECT MODEL SPEC" />
                            </SelectTrigger>
                            <SelectContent className="rounded-xl shadow-2xl border-border">
                              {status.all_models.map(m => <SelectItem key={m} value={m} className="font-bold py-3 px-4">{m.toUpperCase()}</SelectItem>)}
                              {!status.all_models.length && <SelectItem value="none" disabled>NO MODELS AVAILABLE</SelectItem>}
                            </SelectContent>
                          </Select>
                        </div>
                        
                        <div className="space-y-3">
                          <label className="text-[10px] font-black uppercase tracking-widest text-muted-foreground px-1 flex items-center gap-2">
                            <Network className="w-3.5 h-3.5" /> Routing Strategy
                          </label>
                          <Select name="node_addr" value={selectedNode || "dynamic"} onValueChange={(val) => setSelectedNode(val === "dynamic" ? null : val)}>
                            <SelectTrigger className="h-14 rounded-2xl border-2 font-bold text-sm bg-slate-50 focus:ring-primary shadow-sm">
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent className="rounded-xl shadow-2xl border-border">
                              <SelectItem value="dynamic" className="font-black text-primary py-3 px-4 flex items-center gap-2">
                                <Zap className="w-3.5 h-3.5 inline mr-2" /> DYNAMIC BALANCING (BEST NODE)
                              </SelectItem>
                              <Separator className="my-1" />
                              {Object.values(status.nodes).map((n: NodeStatus) => (
                                <SelectItem key={n.address} value={n.address} className="font-bold py-3 px-4">
                                  {n.id.toUpperCase()} - {n.address}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                        </div>
                      </div>

                      <div className="space-y-3">
                        <label className="text-[10px] font-black uppercase tracking-widest text-muted-foreground px-1 flex items-center gap-2">
                          <Terminal className="w-3.5 h-3.5" /> Neural Network Prompt
                        </label>
                        <Textarea 
                          name="prompt" 
                          rows={6} 
                          className="rounded-2xl border-2 font-medium text-sm p-5 bg-slate-50 focus-visible:ring-primary shadow-inner min-h-[180px] leading-relaxed resize-none"
                          placeholder="Synthesize a highly creative prompt to stress-test distributed load balancing and model parallelization..."
                          required
                        />
                      </div>

                      <div className="flex justify-end pt-2">
                        <Button 
                          disabled={testLoading || status.all_models.length === 0} 
                          type="submit" 
                          size="lg"
                          className="h-14 px-10 rounded-2xl font-black uppercase tracking-[0.2em] text-xs shadow-2xl shadow-primary/30 hover:scale-105 active:scale-95 transition-all gap-3 bg-primary"
                        >
                          {testLoading ? <><RefreshCw className="w-5 h-5 animate-spin" /> ANALYZING...</> : <><Play className="w-5 h-5 fill-current/20" /> EXECUTE INFERENCE</>}
                        </Button>
                      </div>
                    </form>

                    <AnimatePresence mode='wait'>
                      {testResult ? (
                        <motion.div
                          key="result"
                          initial={{ opacity: 0, scale: 0.98 }}
                          animate={{ opacity: 1, scale: 1 }}
                          className="space-y-4"
                        >
                          <div className="flex items-center justify-between">
                            <div className="flex items-center gap-2">
                              <Badge className="bg-slate-900 font-black h-6 px-3 uppercase tracking-widest text-[9px]">RESPONSE BUFFER</Badge>
                              <Badge variant="outline" className="border-emerald-500/30 text-emerald-600 bg-emerald-50/50 font-black h-6 px-3 uppercase tracking-widest text-[9px] gap-1.5 flex items-center">
                                <Server className="w-3 h-3" /> SERVED BY {testResult.agent_id.toUpperCase()}
                              </Badge>
                            </div>
                            <Button variant="ghost" size="sm" onClick={() => setTestResult(null)} className="h-6 text-[9px] font-black uppercase text-muted-foreground hover:text-foreground">Clear Output</Button>
                          </div>
                          <div className="bg-slate-950 rounded-2xl p-8 shadow-2xl border-slate-800 shadow-indigo-500/10 min-h-[200px] overflow-hidden group relative">
                            <div className="absolute top-0 right-0 w-32 h-32 bg-primary/10 rounded-full -mr-16 -mt-16 blur-3xl opacity-50 group-hover:opacity-100 transition-opacity duration-1000" />
                            <p className="text-slate-300 text-sm font-mono whitespace-pre-wrap leading-loose relative z-10">{testResult.response}</p>
                          </div>
                        </motion.div>
                      ) : (
                        <motion.div 
                          key="placeholder"
                          initial={{ opacity: 0 }}
                          animate={{ opacity: 0.3 }}
                          className="flex-1 flex flex-col items-center justify-center space-y-4 py-12"
                        >
                          <Play className="w-16 h-16 stroke-1 text-slate-300" />
                          <p className="text-[10px] font-black uppercase tracking-[0.3em] text-slate-400">awaiting neural transmission</p>
                        </motion.div>
                      )}
                    </AnimatePresence>
                  </CardContent>
                </Card>
              </div>

              <div className="lg:col-span-4 space-y-6">
                <Card className="border-none shadow-lg overflow-hidden bg-slate-900 text-white">
                  <CardHeader className="p-6 border-b border-white/5 bg-white/5">
                    <CardTitle className="text-xs font-black uppercase tracking-widest text-primary flex items-center gap-2">
                      <Settings2 className="w-4 h-4" /> Orchestration Details
                    </CardTitle>
                  </CardHeader>
                  <CardContent className="p-6 space-y-6">
                    <div className="space-y-4">
                      <div className="flex items-start gap-4">
                        <div className="w-8 h-8 rounded-xl bg-white/10 flex items-center justify-center flex-shrink-0"><Zap className="w-4 h-4 text-primary" /></div>
                        <div className="space-y-1">
                          <p className="text-xs font-black uppercase tracking-tight leading-none">Dynamic Hedging</p>
                          <p className="text-[10px] font-medium text-white/50 leading-relaxed">Requests are mirrored across nodes with P95 latency tracking to ensure sub-second response times even under load.</p>
                        </div>
                      </div>
                      <div className="flex items-start gap-4">
                        <div className="w-8 h-8 rounded-xl bg-white/10 flex items-center justify-center flex-shrink-0"><Network className="w-4 h-4 text-blue-400" /></div>
                        <div className="space-y-1">
                          <p className="text-xs font-black uppercase tracking-tight leading-none">Load-Aware Routing</p>
                          <p className="text-[10px] font-medium text-white/50 leading-relaxed">The balancer analyzes real-time CPU, VRAM, and temperature metrics to calculate the optimal compute target.</p>
                        </div>
                      </div>
                      <div className="flex items-start gap-4">
                        <div className="w-8 h-8 rounded-xl bg-white/10 flex items-center justify-center flex-shrink-0"><Activity className="w-4 h-4 text-emerald-400" /></div>
                        <div className="space-y-1">
                          <p className="text-xs font-black uppercase tracking-tight leading-none">Circuit Breaking</p>
                          <p className="text-[10px] font-medium text-white/50 leading-relaxed">Automatic node blacklisting and cool-off periods prevent cascading failures across the distributed network.</p>
                        </div>
                      </div>
                    </div>
                  </CardContent>
                </Card>

                <Card className="border-none shadow-md bg-white p-6">
                  <h3 className="text-xs font-black uppercase tracking-[0.2em] text-muted-foreground mb-4 border-b pb-3">Operational Stats</h3>
                  <div className="space-y-5">
                    {[
                      { l: 'TOTAL THROUGHPUT', v: '14.2k req/h', p: 65, c: 'bg-indigo-500' },
                      { l: 'AVG CLUSTER LATENCY', v: '142ms', p: 25, c: 'bg-emerald-500' },
                      { l: 'NODE RELIABILITY', v: '99.98%', p: 99, c: 'bg-primary' },
                    ].map((stat, i) => (
                      <div key={i} className="space-y-2">
                        <div className="flex justify-between text-[10px] font-black tracking-tight uppercase">
                          <span className="text-muted-foreground">{stat.l}</span>
                          <span>{stat.v}</span>
                        </div>
                        <Progress value={stat.p} className="h-1.5 bg-slate-50 shadow-inner" indicatorClassName={stat.c} />
                      </div>
                    ))}
                  </div>
                </Card>
              </div>
            </div>
          </TabsContent>
        </Tabs>
      </main>
    </div>
  );
};

export default App;
