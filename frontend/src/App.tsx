import React, { useState, useEffect, useMemo } from 'react';
import { api } from './api';
import type { ClusterStatus } from './api';
import {
  Server, Database, Trash2, XCircle, Play, RefreshCw, Cpu,
  Activity, CloudDownload, Terminal,
  Network, Zap, Search, MoreVertical, Globe
} from 'lucide-react';
import { motion } from 'framer-motion';
import { Toaster, toast } from 'sonner';

// Shadcn UI Components
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Progress } from "@/components/ui/progress";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Separator } from "@/components/ui/separator";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import {
  DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger,
} from "@/components/ui/dialog";
import {
  Tooltip, TooltipContent, TooltipProvider, TooltipTrigger,
} from "@/components/ui/tooltip";

const Topology = ({ status }: { status: ClusterStatus }) => {
  const nodes = Object.values(status.nodes);
  const centerX = 200;
  const centerY = 150;
  const radius = 110;

  return (
    <div className="w-full h-full min-h-[320px] flex items-center justify-center bg-muted/30 rounded-xl border border-dashed relative overflow-hidden">
      <div className="absolute inset-0 bg-[radial-gradient(var(--border)_1px,transparent_1px)] [background-size:24px_24px] opacity-50" />
      <svg viewBox="0 0 400 300" className="w-full max-w-[500px] h-auto overflow-visible relative z-10">
        <circle cx={centerX} cy={centerY} r="22" className="fill-primary stroke-background stroke-2 shadow-lg" />
        <g transform={`translate(${centerX - 10}, ${centerY - 10})`} className="text-primary-foreground pointer-events-none">
          <Zap size={20} fill="currentColor" fillOpacity={0.2} />
        </g>

        {nodes.map((node, i) => {
          const angle = (i / nodes.length) * 2 * Math.PI - Math.PI / 2;
          const x = centerX + Math.cos(angle) * radius;
          const y = centerY + Math.sin(angle) * radius;
          const isActive = (node.active_models?.length || 0) > 0;

          return (
            <g key={node.address}>
              <line 
                x1={centerX} y1={centerY} x2={x} y2={y} 
                className={`stroke-[1.5] transition-colors duration-500 ${node.state === 2 ? 'stroke-destructive/30' : isActive ? 'stroke-primary' : 'stroke-border'}`} 
                strokeDasharray={node.draining || node.state === 2 ? "4 2" : "0"}
              />
              {isActive && (
                <motion.circle
                  r="2.5" className="fill-primary"
                  animate={{ cx: [centerX, x], cy: [centerY, y] }}
                  transition={{ duration: 1.5, repeat: Infinity, ease: "linear", delay: i * 0.2 }}
                />
              )}
              <circle cx={x} cy={y} r="16" className={`stroke-2 transition-colors duration-500 ${node.has_gpu ? 'fill-indigo-50 stroke-indigo-500' : 'fill-slate-50 stroke-slate-400'}`} />
              <g transform={`translate(${x - 8}, ${y - 8})`} className={node.has_gpu ? "text-indigo-600" : "text-slate-600"}>
                {node.has_gpu ? <Zap size={16} /> : <Cpu size={16} />}
              </g>
              <text x={x} y={y + 28} textAnchor="middle" className="text-[7px] font-black fill-muted-foreground uppercase tracking-tighter">
                {node.id.split('-').pop()}
              </text>
            </g>
          );
        })}
      </svg>
    </div>
  );
};

const App = () => {
  const [status, setStatus] = useState<ClusterStatus | null>(null);
  const [testResult, setTestResult] = useState<{ agent_id: string, response: string } | null>(null);
  const [testLoading, setTestLoading] = useState(false);
  const [logs, setLogs] = useState<string[]>([]);
  const [searchModel, setSearchModel] = useState("");
  const [pullDialogOpen, setPullDialogOpen] = useState(false);
  const [nodePullTarget, setNodePullTarget] = useState<string | null>(null);

  const modelDistribution = useMemo(() => {
    if (!status) return [];
    return status.all_models.map(m => {
      const hostingNodes = Object.values(status.nodes).filter(n => 
        n.active_models?.includes(m) || n.local_models?.some(lm => lm.name === m)
      );
      return {
        name: m,
        nodes: hostingNodes.map(n => ({
          id: n.id,
          address: n.address,
          isHot: n.active_models?.includes(m)
        }))
      };
    });
  }, [status]);

  useEffect(() => {
    const cleanup = api.streamLogs((msg) => {
      setLogs(prev => [...prev.slice(-99), msg]);
    });
    return cleanup;
  }, []);

  const fetchStatus = async () => {
    try {
      const data = await api.getStatus();
      setStatus(data);
    } catch (err) {
      console.error('Connection failure');
    }
  };

  useEffect(() => {
    fetchStatus();
    const interval = setInterval(fetchStatus, 5000);
    return () => clearInterval(interval);
  }, []);

  const handleAction = async (action: () => Promise<void>, msg: string) => {
    const tid = toast.loading('Cluster operation in progress...');
    try {
      await action();
      toast.success(msg, { id: tid });
      fetchStatus();
    } catch {
      toast.error('Action failed', { id: tid });
    }
  };

  const handleTest = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const fd = new FormData(e.currentTarget);
    const model = fd.get('model') as string;
    const prompt = fd.get('prompt') as string;
    const addr = fd.get('node_addr') as string;

    if (model && prompt) {
      setTestLoading(true);
      const tid = toast.loading('Transmitting inference...');
      try {
        const res = addr && addr !== "dynamic"
          ? await api.runTestOnNode(model, prompt, addr)
          : await api.runTest(model, prompt);
        setTestResult(res);
        toast.success(`Served by ${res.agent_id}`, { id: tid });
      } catch {
        toast.error('Transmission error', { id: tid });
      } finally {
        setTestLoading(false);
      }
    }
  };

  if (!status) return (
    <div className="h-screen flex flex-col items-center justify-center gap-4 bg-background">
      <RefreshCw className="animate-spin text-primary" size={32} />
      <span className="font-black text-[10px] uppercase tracking-[0.4em] text-muted-foreground">Syncing Compute Fabric</span>
    </div>
  );

  return (
    <div className="min-h-screen bg-slate-50/30 flex flex-col font-sans selection:bg-primary/10">
      <Toaster position="top-center" richColors closeButton />
      
      <header className="border-b bg-background/80 backdrop-blur-md px-8 h-14 flex items-center justify-between shrink-0 sticky top-0 z-50">
        <div className="flex items-center gap-4">
          <div className="w-8 h-8 rounded-lg bg-primary flex items-center justify-center">
            <Zap className="text-primary-foreground" size={18} fill="currentColor" fillOpacity={0.2} />
          </div>
          <h1 className="text-sm font-black tracking-tighter leading-none">FLAKYOLLAMA <span className="text-primary opacity-50">/</span> ORCHESTRATOR</h1>
        </div>
        
        <div className="flex items-center gap-6 text-[10px] font-bold text-muted-foreground uppercase tracking-widest">
          <div className="flex items-center gap-2">
            <div className="w-1.5 h-1.5 rounded-full bg-emerald-500 animate-pulse" />
            {status.active_workloads} ACTIVE TASKS
          </div>
          <Separator orientation="vertical" className="h-6" />
          <div className="flex items-center gap-2">
            <Button variant="ghost" size="icon" className="h-8 w-8" onClick={fetchStatus}><RefreshCw size={14} /></Button>
          </div>
        </div>
      </header>

      <main className="flex-1 max-w-[1600px] w-full mx-auto p-8 space-y-8">
        
        <div className="grid grid-cols-1 lg:grid-cols-4 gap-8">
          <Card className="lg:col-span-3 border-none shadow-sm bg-background">
            <CardHeader className="py-4 border-b">
              <CardTitle className="text-xs font-black uppercase tracking-widest flex items-center gap-2">
                <Network size={16} className="text-primary" /> Routing Topology
              </CardTitle>
            </CardHeader>
            <CardContent className="p-6">
              <Topology status={status} />
            </CardContent>
          </Card>

          <Card className="border-none shadow-sm bg-background">
            <CardHeader className="py-4 border-b">
              <CardTitle className="text-xs font-black uppercase tracking-widest flex items-center gap-2">
                <Activity size={16} className="text-primary" /> Fabric Health
              </CardTitle>
            </CardHeader>
            <CardContent className="p-6 space-y-6">
              {[
                { label: 'Cluster Nodes', val: Object.keys(status.nodes).length, sub: 'Active Workers', color: 'text-blue-600' },
                { label: 'Network Load', val: status.active_workloads, sub: 'In-Flight', color: 'text-indigo-600' },
                { 
                  label: 'Backlog', 
                  val: status.queue_depth, 
                  sub: 'Pending', 
                  color: status.queue_depth > 0 ? 'text-amber-600' : 'text-slate-400',
                  pulse: status.queue_depth > 0
                },
                { label: 'Model Library', val: status.all_models.length, sub: 'Registered', color: 'text-emerald-600' },
              ].map((kpi, i) => (
                <div key={i} className={`flex items-end justify-between border-b border-dashed pb-4 last:border-0 last:pb-0 ${kpi.pulse ? 'animate-pulse' : ''}`}>
                  <div className="flex flex-col">
                    <span className="text-[9px] font-black uppercase text-muted-foreground tracking-widest">{kpi.label}</span>
                    <span className="text-[10px] font-bold text-muted-foreground/40 uppercase">{kpi.sub}</span>
                  </div>
                  <span className={`text-2xl font-black tracking-tighter ${kpi.color}`}>{kpi.val}</span>
                </div>
              ))}
            </CardContent>
          </Card>
        </div>

        <Card className="border-none shadow-sm bg-background overflow-hidden">
          <CardHeader className="flex flex-row items-center justify-between py-4 border-b">
            <div className="flex items-center gap-2">
              <Database size={16} className="text-primary" />
              <CardTitle className="text-xs font-black uppercase tracking-widest">Distributed Registry</CardTitle>
            </div>
            <div className="flex items-center gap-2">
              <div className="relative w-64 mr-2">
                <Search className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" size={12} />
                <Input 
                  placeholder="FILTER ARCHITECTURE..." 
                  className="pl-9 h-8 border-none bg-muted/50 rounded-lg text-[10px] font-black uppercase"
                  value={searchModel}
                  onChange={e => setSearchModel(e.target.value)}
                />
              </div>
              <Dialog open={pullDialogOpen} onOpenChange={setPullDialogOpen}>
                <DialogTrigger asChild>
                  <Button size="sm" className="h-8 font-black uppercase text-[10px] tracking-widest gap-2">
                    <CloudDownload size={14} /> Pull to Fleet
                  </Button>
                </DialogTrigger>
                <DialogContent>
                  <DialogHeader>
                    <DialogTitle className="font-black tracking-tighter">FLEET DEPLOYMENT</DialogTitle>
                    <DialogDescription className="text-xs font-bold uppercase text-muted-foreground">Broadcast pull command to entire compute fabric</DialogDescription>
                  </DialogHeader>
                  <form onSubmit={(e) => {
                    e.preventDefault();
                    const tag = new FormData(e.currentTarget).get('tag') as string;
                    setPullDialogOpen(false);
                    handleAction(() => api.pullModel(tag), `Syncing ${tag} cluster-wide...`);
                  }} className="space-y-4 pt-4">
                    <Input name="tag" placeholder="e.g. llama3:8b" className="font-bold border-2" required />
                    <Button type="submit" className="w-full font-black uppercase text-xs tracking-widest py-6">Orchestrate Global Sync</Button>
                  </form>
                </DialogContent>
              </Dialog>
            </div>
          </CardHeader>
          <Table>
            <TableHeader className="bg-muted/30">
              <TableRow>
                <TableHead className="text-[10px] font-black uppercase tracking-widest">Architecture</TableHead>
                <TableHead className="text-[10px] font-black uppercase tracking-widest text-center">Compute Distribution (Residency)</TableHead>
                <TableHead className="text-right text-[10px] font-black uppercase tracking-widest">Orchestration</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {modelDistribution.filter(m => m.name.toLowerCase().includes(searchModel.toLowerCase())).map(m => {
                const isSyncing = status.in_progress_pulls && status.in_progress_pulls[m.name];
                return (
                  <TableRow key={m.name} className="group hover:bg-muted/10 transition-colors">
                    <TableCell className="py-4">
                      <div className="flex flex-col">
                        <span className="text-sm font-black tracking-tight">{m.name}</span>
                        {isSyncing ? (
                          <div className="flex items-center gap-2 mt-1">
                            <Badge variant="secondary" className="text-[8px] h-4 font-black bg-indigo-50 text-indigo-600 border-indigo-100 animate-pulse">
                              <RefreshCw size={10} className="animate-spin mr-1" /> SYNCING
                            </Badge>
                          </div>
                        ) : (
                          <span className="text-[9px] font-bold text-muted-foreground uppercase opacity-50">Local Weights Persistent</span>
                        )}
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="flex flex-wrap gap-2 justify-center">
                        {m.nodes.map(n => (
                          <TooltipProvider key={n.address}>
                            <Tooltip>
                              <TooltipTrigger>
                                <Badge 
                                  variant={n.isHot ? "default" : "outline"} 
                                  className={`text-[9px] font-black px-2 h-5 tracking-tighter transition-all ${n.isHot ? 'bg-primary shadow-sm' : 'border-dashed border-muted-foreground/30 text-muted-foreground'}`}
                                >
                                  {n.id.split('-').pop()}
                                  {n.isHot && <div className="w-1 h-1 rounded-full bg-white ml-1.5 animate-pulse" />}
                                </Badge>
                              </TooltipTrigger>
                              <TooltipContent className="text-[10px] font-bold uppercase">
                                {n.id} - {n.isHot ? 'Hot (Resident in VRAM)' : 'Warm (On Disk)'}
                              </TooltipContent>
                            </Tooltip>
                          </TooltipProvider>
                        ))}
                      </div>
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-2 opacity-0 group-hover:opacity-100 transition-opacity">
                        <Button disabled={!!isSyncing} variant="outline" size="icon" className="h-8 w-8 text-amber-600 hover:bg-amber-50" onClick={() => handleAction(() => api.unloadModel(m.name), `Global evict: ${m.name}`)}><XCircle size={14} /></Button>
                        <Button disabled={!!isSyncing} variant="outline" size="icon" className="h-8 w-8 text-destructive hover:bg-destructive hover:text-white" onClick={() => handleAction(() => api.deleteModel(m.name), `Global purge: ${m.name}`)}><Trash2 size={14} /></Button>
                      </div>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </Card>

        <Card className="border-none shadow-sm bg-background">
          <CardHeader className="py-4 border-b">
            <CardTitle className="text-xs font-black uppercase tracking-widest flex items-center gap-2">
              <Server size={16} className="text-primary" /> Infrastructure fleet
            </CardTitle>
          </CardHeader>
          <Table>
            <TableHeader className="bg-muted/30">
              <TableRow>
                <TableHead className="text-[10px] font-black uppercase tracking-widest">Compute Node</TableHead>
                <TableHead className="text-[10px] font-black uppercase tracking-widest">Resources</TableHead>
                <TableHead className="text-[10px] font-black uppercase tracking-widest">Resident Workloads</TableHead>
                <TableHead className="text-right text-[10px] font-black uppercase tracking-widest">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {Object.values(status.nodes).map(node => (
                <TableRow key={node.address} className="hover:bg-muted/10 transition-colors">
                  <TableCell className="py-4">
                    <div className="flex items-center gap-3">
                      <div className={`p-2 rounded-lg ${node.has_gpu ? 'bg-indigo-50 text-indigo-600' : 'bg-slate-100 text-slate-600'}`}>
                        {node.has_gpu ? <Zap size={16} /> : <Cpu size={16} />}
                      </div>
                      <div className="flex flex-col">
                        <span className="text-sm font-black tracking-tight">{node.id}</span>
                        <span className="text-[9px] font-bold text-muted-foreground font-mono">{node.address}</span>
                      </div>
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-col gap-3 w-48">
                      <div className="space-y-1">
                        <div className="flex justify-between text-[8px] font-black text-muted-foreground uppercase"><span>CPU</span> <span>{node.cpu_usage.toFixed(0)}%</span></div>
                        <Progress value={node.cpu_usage} indicatorClassName="bg-blue-500" className="h-1" />
                      </div>
                      <div className="space-y-1">
                        <div className="flex justify-between text-[8px] font-black text-muted-foreground uppercase">
                          <span>{node.has_gpu ? `VRAM` : 'RAM'}</span>
                          <span>{node.has_gpu ? `${(node.vram_used / 1e9).toFixed(1)}G` : `${node.memory_usage.toFixed(0)}%`}</span>
                        </div>
                        <Progress value={node.has_gpu ? (node.vram_used / node.vram_total) * 100 : node.memory_usage} indicatorClassName="bg-emerald-500" className="h-1" />
                      </div>
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-1.5 max-w-[300px]">
                      {node.active_models?.map(m => (
                        <Badge key={m} className="text-[8px] h-4 font-black uppercase tracking-tighter">HOT: {m}</Badge>
                      ))}
                      {node.local_models?.filter(lm => !node.active_models?.includes(lm.name)).map(lm => (
                        <Badge key={lm.name} variant="outline" className="text-[8px] h-4 font-bold border-dashed opacity-60 uppercase tracking-tighter">WARM: {lm.name}</Badge>
                      ))}
                    </div>
                  </TableCell>
                  <TableCell className="text-right">
                    <div className="flex flex-col items-end gap-2">
                      <div className="flex items-center gap-2">
                        {node.state === 0 ? (
                          <Badge variant="secondary" className="text-[9px] font-black h-5 uppercase bg-emerald-50 text-emerald-700 border-emerald-100">READY</Badge>
                        ) : node.state === 1 ? (
                          <TooltipProvider>
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <Badge variant="outline" className="text-[9px] font-black h-5 uppercase bg-amber-50 text-amber-700 border-amber-200">DEGRADED</Badge>
                              </TooltipTrigger>
                              <TooltipContent>Recent errors detected: {node.errors}</TooltipContent>
                            </Tooltip>
                          </TooltipProvider>
                        ) : (
                          <Badge variant="destructive" className="text-[9px] font-black h-5 uppercase">OFFLINE</Badge>
                        )}
                        {node.draining && <Badge className="text-[9px] font-black h-5 bg-amber-500 text-white">DRAINING</Badge>}
                      </div>
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild><Button variant="ghost" size="icon" className="h-7 w-7"><MoreVertical size={14} /></Button></DropdownMenuTrigger>
                        <DropdownMenuContent align="end" className="font-black text-[10px] uppercase tracking-widest text-foreground">
                          <DropdownMenuItem onClick={() => handleAction(() => node.draining ? api.undrainNode(node.address) : api.drainNode(node.address), `State updated`)}>
                            {node.draining ? 'RESUME TRAFFIC' : 'DRAIN NODE'}
                          </DropdownMenuItem>
                          <DropdownMenuItem onClick={() => setNodePullTarget(node.address)}>DEPLOY TO NODE</DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Card>

        <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
          <Card className="border-none shadow-sm bg-background overflow-hidden flex flex-col">
            <CardHeader className="py-4 border-b bg-muted/10">
              <CardTitle className="text-xs font-black uppercase tracking-widest flex items-center gap-2"><Terminal size={16} className="text-primary" /> Inference Playground</CardTitle>
            </CardHeader>
            <CardContent className="p-6">
              <form onSubmit={handleTest} className="space-y-6">
                <div className="grid grid-cols-2 gap-6">
                  <div className="space-y-2">
                    <label className="text-[9px] font-black uppercase text-muted-foreground px-1 tracking-widest">Neural architecture</label>
                    <Select name="model" required defaultValue={status.all_models[0]}>
                      <SelectTrigger className="h-10 font-bold text-xs border-2"><SelectValue placeholder="Target" /></SelectTrigger>
                      <SelectContent className="font-bold text-xs">
                        {status.all_models.map(m => <SelectItem key={m} value={m}>{m.toUpperCase()}</SelectItem>)}
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="space-y-2">
                    <label className="text-[9px] font-black uppercase text-muted-foreground px-1 tracking-widest">Compute provider</label>
                    <Select name="node_addr" defaultValue="dynamic">
                      <SelectTrigger className="h-10 font-bold text-xs border-2"><SelectValue /></SelectTrigger>
                      <SelectContent className="font-bold text-xs">
                        <SelectItem value="dynamic">DYNAMIC BALANCING</SelectItem>
                        {Object.values(status.nodes).map(n => <SelectItem key={n.address} value={n.address}>{n.id.toUpperCase()}</SelectItem>)}
                      </SelectContent>
                    </Select>
                  </div>
                </div>
                <Textarea name="prompt" placeholder="TRANSMIT PROMPT..." className="min-h-[120px] text-xs font-bold uppercase p-4 border-2 resize-none" required />
                <Button type="submit" disabled={testLoading} className="w-full font-black text-xs tracking-[0.3em] uppercase py-6 shadow-xl shadow-primary/20">
                  {testLoading ? <RefreshCw className="animate-spin mr-3" size={16} /> : <Play className="mr-3" size={16} fill="currentColor" fillOpacity={0.2} />}
                  Execute Inference
                </Button>
              </form>
            </CardContent>
          </Card>

          <Card className="border-none shadow-sm bg-background overflow-hidden flex flex-col">
            <CardHeader className="py-4 border-b">
              <CardTitle className="text-xs font-black uppercase tracking-widest">Inference Response Buffer</CardTitle>
            </CardHeader>
            <CardContent className="p-0 flex-1 relative min-h-[300px]">
              <ScrollArea className="h-full bg-slate-950 p-6">
                <div className="text-slate-300 text-xs font-mono whitespace-pre-wrap leading-relaxed">
                  {testResult ? (
                    <div>
                      <div className="border-b border-white/10 pb-4 mb-4 flex items-center justify-between">
                        <Badge variant="outline" className="bg-emerald-500/10 text-emerald-500 border-emerald-500/20 text-[9px] font-black uppercase tracking-widest">Provider: {testResult.agent_id}</Badge>
                        <Button variant="ghost" size="sm" onClick={() => setTestResult(null)} className="h-6 text-[8px] font-black uppercase text-slate-500">Clear Buffer</Button>
                      </div>
                      {testResult.response}
                    </div>
                  ) : (
                    <div className="h-full flex flex-col items-center justify-center pt-20 opacity-20 grayscale select-none">
                      <Globe className="animate-pulse mb-4" size={48} />
                      <span className="text-[10px] font-black uppercase tracking-[0.5em]">Awaiting Transmission</span>
                    </div>
                  )}
                </div>
              </ScrollArea>
            </CardContent>
          </Card>
        </div>

        <Card className="border-none shadow-sm bg-slate-950">
          <CardHeader className="py-3 px-6 border-b border-white/5 flex flex-row items-center justify-between">
            <div className="flex items-center gap-2">
              <div className="w-2 h-2 rounded-full bg-primary animate-pulse" />
              <CardTitle className="text-[9px] font-black uppercase tracking-[0.3em] text-slate-400">Live Telemetry Stream</CardTitle>
            </div>
            <Button variant="ghost" size="sm" onClick={() => setLogs([])} className="h-6 text-[8px] font-black uppercase text-slate-500 hover:text-white">Flush buffer</Button>
          </CardHeader>
          <CardContent className="p-0">
            <ScrollArea className="h-[200px] font-mono text-[10px] text-indigo-300/70">
              <div className="p-6 flex flex-col-reverse gap-1.5">
                {[...logs].reverse().map((log, i) => (
                  <div key={i} className="flex gap-4 border-l border-primary/20 pl-4 py-0.5 hover:bg-white/5 transition-all">
                    <span className="text-slate-600 shrink-0 font-black tracking-tighter">[{new Date().toLocaleTimeString()}]</span>
                    <p className="tracking-tight">{log}</p>
                  </div>
                ))}
              </div>
            </ScrollArea>
          </CardContent>
        </Card>
      </main>

      {/* Node-Specific Pull Dialog */}
      <Dialog open={!!nodePullTarget} onOpenChange={(open) => !open && setNodePullTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="font-black tracking-tighter text-foreground uppercase">Node-Specific Deployment</DialogTitle>
            <DialogDescription className="text-xs font-bold uppercase text-muted-foreground">Targeting worker: {nodePullTarget}</DialogDescription>
          </DialogHeader>
          <form onSubmit={(e) => {
            e.preventDefault();
            const tag = new FormData(e.currentTarget).get('tag') as string;
            if (nodePullTarget) {
              handleAction(() => api.pullModel(tag, nodePullTarget), `Deploying ${tag} to ${nodePullTarget}...`);
              setNodePullTarget(null);
            }
          }} className="space-y-4 pt-4">
            <Input name="tag" placeholder="e.g. mistral" className="font-bold border-2" required />
            <Button type="submit" className="w-full font-black uppercase text-xs tracking-widest py-6">Initiate Targeted Pull</Button>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
};

export default App;
