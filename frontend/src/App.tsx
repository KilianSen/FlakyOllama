import React, { useState, useEffect, useRef } from 'react';
import { api, type NodeStatus } from './api';
import type { ClusterStatus } from './api';
import {
  Server, Database, Trash2, XCircle, Play, Layers, RefreshCw, Cpu,
  Activity, AlertTriangle, CheckCircle2, CloudDownload, Terminal,
  Network, Zap, HardDrive, Info, Settings2, Search, MoreVertical,
  Maximize2, Minimize2, Trash
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
import { 
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, 
  AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle, 
  AlertDialogTrigger 
} from "@/components/ui/alert-dialog";
import { 
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, 
  DialogTitle, DialogTrigger 
} from "@/components/ui/dialog";

const ClusterTopology = ({ status }: { status: ClusterStatus }) => {
  const nodeEntries = Object.entries(status.nodes);
  const radius = 120;

  return (
    <div className="relative h-[320px] w-full flex items-center justify-center pointer-events-none">
      {/* Central Hub */}
      <motion.div 
        animate={{ scale: [1, 1.05, 1] }}
        transition={{ duration: 4, repeat: Infinity }}
        className="relative z-20 w-12 h-12 bg-primary rounded-2xl flex items-center justify-center shadow-2xl shadow-primary/20 border-2 border-background"
      >
        <Zap className="w-6 h-6 text-primary-foreground fill-primary-foreground/20" />
      </motion.div>

      {/* Nodes and Connections */}
      {nodeEntries.map(([addr, node], i) => {
        const angle = (i / nodeEntries.length) * 2 * Math.PI - Math.PI / 2;
        const x = Math.cos(angle) * radius;
        const y = Math.sin(angle) * radius;
        const isActive = (node.active_models?.length || 0) > 0;

        return (
          <div key={addr} className="absolute inset-0 flex items-center justify-center">
            <svg className="absolute inset-0 w-full h-full pointer-events-none overflow-visible">
              <motion.line
                initial={{ pathLength: 0, opacity: 0 }}
                animate={{ pathLength: 1, opacity: 1 }}
                x1="50%" y1="50%"
                x2={`calc(50% + ${x}px)`} y2={`calc(50% + ${y}px)`}
                stroke={node.draining ? "var(--warning)" : isActive ? "var(--primary)" : "var(--border)"}
                strokeWidth={isActive ? "1.5" : "1"}
                strokeDasharray={node.draining ? "3 3" : "0"}
                className={node.draining ? "stroke-amber-500/50" : isActive ? "stroke-primary/50" : "stroke-muted-foreground/20"}
              />
            </svg>

            <motion.div
              style={{ transform: `translate(${x}px, ${y}px)` }}
              className="absolute pointer-events-auto"
            >
              <div className={`w-8 h-8 rounded-xl flex items-center justify-center shadow-sm border-2 transition-colors ${
                node.draining ? 'bg-amber-50 border-amber-300' : isActive ? 'bg-background border-primary' : 'bg-background border-border'
              }`}>
                {node.has_gpu ? (
                  <Zap className={`w-4 h-4 ${node.draining ? 'text-amber-500' : isActive ? 'text-primary' : 'text-muted-foreground'}`} />
                ) : (
                  <Cpu className={`w-4 h-4 ${node.draining ? 'text-amber-500' : isActive ? 'text-primary' : 'text-muted-foreground'}`} />
                )}
              </div>
            </motion.div>
          </div>
        );
      })}
    </div>
  );
};

const NodeMinimalCard = ({ node, onAction }: { node: NodeStatus, onAction: (a: string, d: boolean) => void }) => (
  <div className="flex items-center justify-between p-3 rounded-xl border bg-card hover:bg-muted/30 transition-colors group">
    <div className="flex items-center gap-3">
      <div className={`p-2 rounded-lg ${node.has_gpu ? 'bg-indigo-50 text-indigo-600' : 'bg-slate-100 text-slate-600'}`}>
        {node.has_gpu ? <Zap className="w-4 h-4" /> : <Cpu className="w-4 h-4" />}
      </div>
      <div className="flex flex-col">
        <div className="flex items-center gap-2">
          <span className="text-sm font-bold truncate max-w-[120px]">{node.id}</span>
          <Badge variant={node.state === 0 ? "secondary" : "destructive"} className="h-4 px-1 text-[8px] font-black uppercase tracking-tighter">
            {node.state === 0 ? 'HLTH' : 'FAIL'}
          </Badge>
        </div>
        <span className="text-[10px] text-muted-foreground font-mono">{node.address}</span>
      </div>
    </div>
    
    <div className="flex items-center gap-4">
      <div className="flex flex-col items-end min-w-[60px]">
        <span className="text-[9px] font-black uppercase text-muted-foreground">Load</span>
        <span className="text-xs font-bold">{node.cpu_usage.toFixed(0)}%</span>
      </div>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" size="icon" className="h-8 w-8 opacity-0 group-hover:opacity-100"><MoreVertical className="w-4 h-4" /></Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="font-bold text-[10px] uppercase tracking-tighter">
          <DropdownMenuItem onClick={() => onAction(node.address, node.draining)}>
            {node.draining ? 'RESUME TRAFFIC' : 'DRAIN NODE'}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  </div>
);

const App = () => {
  const [status, setStatus] = useState<ClusterStatus | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [testResult, setTestResult] = useState<{ agent_id: string, response: string } | null>(null);
  const [testLoading, setTestLoading] = useState(false);
  const [logs, setLogs] = useState<string[]>([]);
  const [showLogs, setShowLogs] = useState(true);
  const [selectedNode, setSelectedNode] = useState<string | null>(null);
  const [searchModel, setSearchModel] = useState("");
  const [isPulling, setIsPulling] = useState(false);
  const logsEndRef = useRef<HTMLDivElement>(null);

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
      if (error) setError(null);
    } catch (err) {
      if (!error && !silent) setError('Connection lost');
    }
  };

  useEffect(() => {
    fetchStatus();
    const interval = setInterval(() => fetchStatus(true), 5000);
    return () => clearInterval(interval);
  }, [error]);

  const handleAction = async (action: () => Promise<void>, msg: string) => {
    const toastId = toast.loading('Processing...');
    try {
      await action();
      toast.success(msg, { id: toastId });
      fetchStatus(true);
    } catch (err) {
      toast.error('Action failed', { id: toastId });
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
      const toastId = toast.loading('Analyzing...');
      try {
        const res = nodeAddr && nodeAddr !== "dynamic"
          ? await api.runTestOnNode(model, prompt, nodeAddr)
          : await api.runTest(model, prompt);
        setTestResult(res);
        toast.success(`Served by ${res.agent_id}`, { id: toastId });
      } catch (err) {
        toast.error('Transmission error', { id: toastId });
      } finally {
        setTestLoading(false);
      }
    }
  };

  if (!status) return (
    <div className="flex items-center justify-center min-h-screen bg-background">
      <div className="flex flex-col items-center gap-4 animate-pulse">
        <div className="w-12 h-12 bg-primary rounded-2xl flex items-center justify-center shadow-2xl shadow-primary/30">
          <Zap className="w-6 h-6 text-primary-foreground fill-current/20" />
        </div>
        <span className="text-[10px] font-black uppercase tracking-[0.3em] text-muted-foreground">Connecting Orchestrator</span>
      </div>
    </div>
  );

  const filteredModels = status.all_models.filter(m => m.toLowerCase().includes(searchModel.toLowerCase()));

  return (
    <div className="flex h-screen bg-slate-50/50 text-foreground font-sans overflow-hidden">
      <Toaster position="top-center" richColors />

      {/* Main Integrated Workspace */}
      <div className="flex-1 flex flex-col min-w-0 border-r relative bg-background">
        {/* Unified Top Header */}
        <header className="h-14 border-b flex items-center justify-between px-6 bg-background/80 backdrop-blur-md sticky top-0 z-30">
          <div className="flex items-center gap-3">
            <div className="w-8 h-8 rounded-xl bg-primary flex items-center justify-center shadow-lg shadow-primary/20">
              <Zap className="w-4 h-4 text-primary-foreground" />
            </div>
            <h1 className="text-sm font-black tracking-tighter uppercase flex items-center gap-2">
              FlakyOllama
              <Badge variant="outline" className="text-[8px] font-black h-4 px-1 leading-none uppercase">v2.1</Badge>
            </h1>
          </div>
          
          <div className="flex items-center gap-4">
            <div className="flex items-center gap-2 text-[10px] font-black text-muted-foreground">
              <div className="w-1.5 h-1.5 rounded-full bg-emerald-500 animate-pulse" />
              {status.active_workloads} ACTIVE
            </div>
            <Separator orientation="vertical" className="h-6" />
            <Button variant="ghost" size="icon" onClick={() => fetchStatus(false)} className="h-8 w-8"><RefreshCw className="w-4 h-4" /></Button>
          </div>
        </header>

        {/* Dynamic Canvas Area */}
        <div className="flex-1 overflow-y-auto custom-scrollbar">
          <div className="max-w-5xl mx-auto p-8 space-y-12">
            
            {/* Visual Topology Header */}
            <section className="relative rounded-3xl border bg-card/50 shadow-inner overflow-hidden">
              <div className="absolute top-6 left-6 z-10 flex flex-col gap-1">
                <h2 className="text-xs font-black uppercase tracking-widest text-muted-foreground">Cluster Topology</h2>
                <div className="flex items-center gap-3">
                  <span className="text-2xl font-black tracking-tighter">{Object.keys(status.nodes).length} Workers</span>
                  <Badge variant="outline" className="text-[9px] border-emerald-500/20 text-emerald-600 bg-emerald-50">SYNCED</Badge>
                </div>
              </div>
              <ClusterTopology status={status} />
            </section>

            {/* Integrated Controls Grid */}
            <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
              
              {/* Models Section */}
              <div className="space-y-4">
                <div className="flex items-center justify-between px-1">
                  <h3 className="text-[10px] font-black uppercase tracking-widest text-muted-foreground">Local Model Registry</h3>
                  <Dialog>
                    <DialogTrigger asChild>
                      <Button variant="outline" size="sm" className="h-7 text-[9px] font-black uppercase px-3 rounded-full">Deploy New</Button>
                    </DialogTrigger>
                    <DialogContent className="sm:max-w-[425px] rounded-2xl">
                      <DialogHeader>
                        <DialogTitle className="text-xl font-black tracking-tighter">FLEET DEPLOYMENT</DialogTitle>
                        <DialogDescription className="text-xs font-bold uppercase tracking-wider text-muted-foreground">Specify ollama tag to orchestrate across fleet</DialogDescription>
                      </DialogHeader>
                      <form onSubmit={(e) => {
                        e.preventDefault();
                        const tag = new FormData(e.currentTarget).get('tag') as string;
                        handleAction(() => api.pullModel(tag), `Orchestrating ${tag} pull...`);
                      }} className="space-y-4 pt-4">
                        <Input name="tag" placeholder="e.g. llama3:8b" className="h-12 rounded-xl border-2 font-bold" required />
                        <DialogFooter>
                          <Button type="submit" className="w-full h-12 rounded-xl font-black uppercase tracking-widest text-xs">Execute Global Pull</Button>
                        </DialogFooter>
                      </form>
                    </DialogContent>
                  </Dialog>
                </div>
                
                <Card className="rounded-2xl border bg-muted/5 shadow-none overflow-hidden">
                  <div className="p-2">
                    <div className="relative mb-2">
                      <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-muted-foreground" />
                      <Input 
                        placeholder="FILTER REGISTRY..." 
                        value={searchModel} 
                        onChange={(e) => setSearchModel(e.target.value)} 
                        className="pl-9 h-9 border-none bg-muted/50 rounded-lg text-xs font-bold uppercase tracking-tight shadow-none focus-visible:ring-primary/20"
                      />
                    </div>
                    <ScrollArea className="h-[280px]">
                      <div className="space-y-1">
                        {filteredModels.map(m => (
                          <div key={m} className="flex items-center justify-between p-2 rounded-lg hover:bg-muted/50 group transition-colors">
                            <div className="flex flex-col">
                              <span className="text-[11px] font-bold tracking-tight">{m}</span>
                              <span className="text-[8px] font-black uppercase text-muted-foreground tracking-tighter">Ollama Local</span>
                            </div>
                            <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                              <Button variant="ghost" size="icon" className="h-7 w-7 text-amber-600" onClick={() => handleAction(() => api.unloadModel(m), `Evicted ${m}`)}><XCircle className="w-3.5 h-3.5" /></Button>
                              <AlertDialog>
                                <AlertDialogTrigger asChild>
                                  <Button variant="ghost" size="icon" className="h-7 w-7 text-destructive"><Trash2 className="w-3.5 h-3.5" /></Button>
                                </AlertDialogTrigger>
                                <AlertDialogContent className="rounded-2xl border-2">
                                  <AlertDialogHeader>
                                    <AlertDialogTitle className="font-black uppercase tracking-widest text-sm">Purge Weights</AlertDialogTitle>
                                    <AlertDialogDescription className="text-xs font-bold text-muted-foreground">
                                      This will permanently remove <strong>{m}</strong> from all cluster nodes. This action is irreversible.
                                    </AlertDialogDescription>
                                  </AlertDialogHeader>
                                  <AlertDialogFooter>
                                    <AlertDialogCancel className="rounded-xl font-bold">CANCEL</AlertDialogCancel>
                                    <AlertDialogAction onClick={() => handleAction(() => api.deleteModel(m), `Purged ${m}`)} className="bg-destructive hover:bg-destructive/90 rounded-xl font-black uppercase text-[10px]">SCRUB WEIGHTS</AlertDialogAction>
                                  </AlertDialogFooter>
                                </AlertDialogContent>
                              </AlertDialog>
                            </div>
                          </div>
                        ))}
                      </div>
                    </ScrollArea>
                  </div>
                </Card>
              </div>

              {/* Workers Section */}
              <div className="space-y-4">
                <div className="flex items-center justify-between px-1">
                  <h3 className="text-[10px] font-black uppercase tracking-widest text-muted-foreground">Fleet Infrastructure</h3>
                  <Badge variant="outline" className="h-5 px-2 text-[8px] font-black uppercase tracking-tighter leading-none bg-background shadow-sm">{Object.keys(status.nodes).length} NODES</Badge>
                </div>
                <div className="space-y-3">
                  {Object.values(status.nodes).map(node => (
                    <NodeMinimalCard key={node.address} node={node} onAction={(a, d) => handleAction(() => d ? api.undrainNode(a) : api.drainNode(a), `Node status updated`)} />
                  ))}
                </div>
              </div>

            </div>

            {/* Integrated Playground Panel */}
            <section className="space-y-4">
              <div className="flex items-center gap-3 px-1">
                <div className="p-1.5 bg-indigo-50 text-indigo-600 rounded-lg"><Activity className="w-4 h-4" /></div>
                <h3 className="text-sm font-black uppercase tracking-tight">Active Inference Terminal</h3>
              </div>
              <Card className="rounded-2xl border bg-background shadow-lg overflow-hidden flex flex-col min-h-[400px]">
                <form onSubmit={handleTest} className="p-6 border-b bg-muted/10 space-y-6">
                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-6">
                    <div className="space-y-2">
                      <label className="text-[10px] font-black uppercase tracking-widest text-muted-foreground px-1">Neural Target</label>
                      <Select name="model" required defaultValue={status.all_models[0]}>
                        <SelectTrigger className="h-11 rounded-xl font-bold text-xs bg-background shadow-sm border-2">
                          <SelectValue placeholder="SELECT MODEL" />
                        </SelectTrigger>
                        <SelectContent className="rounded-xl font-bold text-xs">
                          {status.all_models.map(m => <SelectItem key={m} value={m}>{m.toUpperCase()}</SelectItem>)}
                        </SelectContent>
                      </Select>
                    </div>
                    <div className="space-y-2">
                      <label className="text-[10px] font-black uppercase tracking-widest text-muted-foreground px-1">Compute Routing</label>
                      <Select name="node_addr" defaultValue="dynamic">
                        <SelectTrigger className="h-11 rounded-xl font-bold text-xs bg-background shadow-sm border-2">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent className="rounded-xl font-bold text-xs">
                          <SelectItem value="dynamic">DYNAMIC BALANCING</SelectItem>
                          <Separator className="my-1" />
                          {Object.values(status.nodes).map(n => <SelectItem key={n.address} value={n.address}>{n.id.toUpperCase()}</SelectItem>)}
                        </SelectContent>
                      </Select>
                    </div>
                  </div>
                  <div className="space-y-2">
                    <Textarea 
                      name="prompt" 
                      placeholder="Input inference prompt..." 
                      className="min-h-[100px] rounded-xl font-medium text-sm p-4 bg-background border-2 shadow-inner resize-none focus-visible:ring-primary/20"
                      required
                    />
                  </div>
                  <div className="flex justify-end">
                    <Button type="submit" disabled={testLoading || !status.all_models.length} className="h-11 px-8 rounded-xl font-black uppercase tracking-widest text-[10px] bg-indigo-600 hover:bg-indigo-700 shadow-xl shadow-indigo-200">
                      {testLoading ? <RefreshCw className="w-4 h-4 animate-spin mr-2" /> : <Play className="w-4 h-4 mr-2 fill-current/20" />}
                      Execute Inference
                    </Button>
                  </div>
                </form>
                <div className="flex-1 p-6 font-mono text-xs leading-relaxed overflow-hidden">
                  {testResult ? (
                    <div className="space-y-4 h-full flex flex-col">
                      <div className="flex items-center gap-2">
                        <Badge variant="outline" className="text-[9px] font-black uppercase tracking-tighter bg-emerald-50 text-emerald-700 border-emerald-200 leading-none h-5">Response: {testResult.agent_id}</Badge>
                        <Button variant="ghost" size="sm" onClick={() => setTestResult(null)} className="h-5 text-[8px] font-bold p-0 px-2 opacity-50">Clear</Button>
                      </div>
                      <ScrollArea className="flex-1 p-4 bg-slate-950 text-slate-300 rounded-xl border border-slate-800 shadow-inner">
                        <div className="whitespace-pre-wrap leading-relaxed">{testResult.response}</div>
                      </ScrollArea>
                    </div>
                  ) : (
                    <div className="h-full flex flex-col items-center justify-center opacity-20 select-none grayscale scale-75">
                      <Terminal className="w-12 h-12 mb-4" />
                      <span className="text-[10px] font-black uppercase tracking-[0.4em]">System Idle</span>
                    </div>
                  )}
                </div>
              </Card>
            </section>

          </div>
        </div>
      </div>

      {/* Integrated Live Telemetry Sidebar */}
      <div className={`w-[320px] bg-slate-950 border-l border-slate-800 flex flex-col shadow-2xl z-20 transition-all duration-300 ${showLogs ? 'mr-0' : '-mr-[320px]'}`}>
        <header className="h-14 border-b border-slate-800 flex items-center justify-between px-5">
          <div className="flex items-center gap-2">
            <Terminal className="w-4 h-4 text-primary" />
            <span className="text-[10px] font-black uppercase tracking-widest text-slate-400">Telemetry Stream</span>
          </div>
          <Button variant="ghost" size="icon" onClick={() => setLogs([])} className="h-8 w-8 text-slate-500 hover:text-white"><Trash className="w-3.5 h-3.5" /></Button>
        </header>
        <ScrollArea className="flex-1 font-mono text-[10px] text-indigo-300/80 p-0">
          <div className="p-5 flex flex-col gap-2">
            {logs.length === 0 && <div className="text-center py-20 opacity-20 uppercase font-black tracking-widest text-[8px]">Waiting for events...</div>}
            {[...logs].reverse().map((log, i) => (
              <div key={i} className="group border-l-2 border-primary/20 pl-3 py-1 hover:border-primary hover:bg-white/5 transition-all">
                <span className="text-slate-600 block mb-0.5 text-[8px] font-black">[{new Date().toLocaleTimeString()}]</span>
                <p className="group-hover:text-primary transition-colors">{log}</p>
              </div>
            ))}
          </div>
        </ScrollArea>
        <footer className="p-4 border-t border-slate-800 bg-slate-900/50">
          <div className="bg-slate-950 rounded-lg p-3 border border-slate-800 flex items-center justify-between">
            <div className="flex flex-col gap-0.5">
              <span className="text-[8px] font-black text-slate-500 uppercase tracking-tighter">Connection Status</span>
              <span className="text-[10px] font-black text-emerald-500 uppercase tracking-widest flex items-center gap-1.5">
                <div className="w-1 h-1 rounded-full bg-emerald-500 shadow-[0_0_8px_rgba(16,185,129,0.8)]" /> Active
              </span>
            </div>
            <div className="flex flex-col items-end gap-0.5">
              <span className="text-[8px] font-black text-slate-500 uppercase tracking-tighter">Event Buffer</span>
              <span className="text-[10px] font-black text-slate-300 uppercase tracking-widest">{logs.length}/100</span>
            </div>
          </div>
        </footer>
      </div>

      {/* Floating Panel Toggle */}
      <div className="fixed bottom-6 right-6 z-50">
        <Button 
          variant="secondary" 
          size="icon" 
          onClick={() => setShowLogs(!showLogs)} 
          className="h-12 w-12 rounded-2xl shadow-2xl border-2 border-background shadow-primary/20 hover:scale-110 active:scale-95 transition-all"
        >
          <Terminal className={`w-5 h-5 transition-transform duration-500 ${showLogs ? 'rotate-180' : ''}`} />
        </Button>
      </div>
    </div>
  );
};

export default App;
