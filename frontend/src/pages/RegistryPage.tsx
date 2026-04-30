import React, { useState, useEffect, useMemo } from 'react';
import {
  Search, Download, Trash2, CheckCircle2, Box, RefreshCw, ShieldX, Pin, Zap, Clock, ChevronRight, TrendingUp, Cpu
} from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { Badge } from '@/components/ui/badge';
import { Tabs, TabsContent } from '@/components/ui/tabs';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { toast } from 'sonner';
import sdk, { type ModelRequest, type NodeStatus } from '../api';
import { useCluster } from '../ClusterContext';

// Common models for the browser
const POPULAR_MODELS = [
  { name: 'llama3.2:1b', size: '1.3GB', desc: 'Meta Llama 3.2 1B - ultra lightweight and fast.', family: 'llama' },
  { name: 'llama3:8b', size: '4.7GB', desc: 'Meta Llama 3 8B - the most capable open-source model at this scale.', family: 'llama' },
  { name: 'mistral:7b', size: '4.1GB', desc: 'Mistral 7B - high-performance transformer model.', family: 'mistral' },
  { name: 'phi3:latest', size: '2.3GB', desc: 'Microsoft Phi-3 Mini - 3.8B parameter lightweight model.', family: 'phi' },
  { name: 'gemma2:2b', size: '1.6GB', desc: 'Google Gemma 2 - lightweight, state-of-the-art open models.', family: 'gemma' },
];

export const RegistryPage: React.FC = () => {
  const { status, refresh } = useCluster();
  const [search, setSearch] = useState('');
  const [requests, setRequests] = useState<ModelRequest[]>([]);
  const [activeTab, setActiveTab] = useState('matrix');

  const nodes = useMemo(() => Object.values(status?.nodes || {}), [status]);
  const virtualModels = useMemo(() => status?.virtual_models || {}, [status]);
  const virtualModelNames = useMemo(() => Object.keys(virtualModels).sort(), [virtualModels]);
  
  // Physical models are all_models minus virtual_models
  const physicalModelNames = useMemo(() => {
    const all = status?.all_models || [];
    return all.filter(m => !virtualModels[m]).sort();
  }, [status, virtualModels]);
  
  const loadRequests = async () => {
    try {
      const data = await sdk.getModelRequests('pending');
      setRequests(data || []);
    } catch (err) {
      setRequests([]);
    }
  };

  useEffect(() => {
    loadRequests();
    const interval = setInterval(loadRequests, 10000);
    return () => clearInterval(interval);
  }, []);

  const handleApprove = async (id: string) => {
    try {
      await sdk.approveModelRequest(id);
      toast.success('Approved');
      loadRequests();
      refresh();
    } catch (err: any) { toast.error(err.message); }
  };

  const handleDecline = async (id: string) => {
    try {
      await sdk.declineModelRequest(id);
      toast.success('Declined');
      loadRequests();
    } catch (err: any) { toast.error(err.message); }
  };

  const handlePull = async (model: string, targetNode?: string) => {
    try {
      const res = await sdk.pullModel(model, targetNode);
      if (res.status === 'approval_pending') {
        toast.info('Request submitted for approval');
        loadRequests();
      } else {
        toast.success(`Pull triggered for ${model}`);
        refresh();
      }
    } catch (err: any) { toast.error(err.message); }
  };

  const handleDelete = async (model: string, nodeID?: string) => {
    try {
      // Note: Backend might need update to support node-specific delete via SDK
      await sdk.deleteModel(model);
      toast.success(`Delete triggered for ${model}${nodeID ? ` on ${nodeID}` : ''}`);
      refresh();
    } catch (err: any) { toast.error(err.message); }
  };

  const togglePolicy = async (model: string, nodeID: string, type: 'banned' | 'pinned' | 'persistent') => {
    const current = status?.model_policies?.[model]?.[nodeID] || { Banned: false, Pinned: false, Persistent: false };
    const nextBanned = type === 'banned' ? !current.Banned : current.Banned;
    const nextPinned = type === 'pinned' ? !current.Pinned : current.Pinned;
    const nextPersistent = type === 'persistent' ? !current.Persistent : current.Persistent;

    try {
      await sdk.setModelPolicy(model, nodeID, nextBanned, nextPinned, nextPersistent);
      toast.success('Policy updated');
      refresh();
    } catch (err: any) { toast.error(err.message); }
  };

  const getModelStatusOnNode = (model: string, node: NodeStatus) => {
    const isLoaded = (node.active_models || []).includes(model);
    const isOnDisk = (node.local_models || []).some(m => m.model === model);
    const policy = status?.model_policies?.[model]?.[node.id] || { Banned: false, Pinned: false, Persistent: false };
    
    return { isLoaded, isOnDisk, isBanned: policy.Banned, isPinned: policy.Pinned, isPersistent: policy.Persistent };
  };

  const filteredPhysicalModels = physicalModelNames.filter(m => m.toLowerCase().includes(search.toLowerCase()));
  const filteredVirtualModels = virtualModelNames.filter(m => m.toLowerCase().includes(search.toLowerCase()));

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-sm font-black uppercase tracking-widest">Fleet Registry</h2>
          <p className="text-[11px] text-muted-foreground mt-0.5">Global model state and per-node availability matrix</p>
        </div>
        <div className="flex items-center gap-3">
          <Button variant="outline" size="sm" className="h-8 text-[10px] font-black uppercase" onClick={refresh}>
            <RefreshCw size={12} className="mr-2" /> Refresh State
          </Button>
          {requests.length > 0 && (
            <Badge className="bg-amber-500/20 text-amber-400 border-amber-500/30 font-black animate-pulse">
              {requests.length} Requests
            </Badge>
          )}
        </div>
      </div>

      <Tabs value={activeTab} onValueChange={setActiveTab} className="space-y-4">
        <div className="flex bg-muted/30 p-1 rounded-lg border border-border/50 w-fit">
           {['matrix', 'virtual', 'browser', 'requests'].map(t => (
             <button 
              key={t}
              onClick={() => setActiveTab(t)}
              className={`px-4 py-1.5 text-[10px] font-black uppercase tracking-wider rounded-md transition-all flex items-center gap-2 ${activeTab === t ? 'bg-background text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground'}`}
             >
               {t === 'matrix' && <Layers size={12} />}
               {t === 'virtual' && <Cpu size={12} />}
               {t === 'browser' && <Search size={12} />}
               {t === 'requests' && <Clock size={12} />}
               {t}
               {t === 'requests' && requests.length > 0 && <span className="w-1.5 h-1.5 rounded-full bg-amber-500" />}
             </button>
           ))}
        </div>

        {/* ── Tab: Matrix (Physical Models) ── */}
        <TabsContent value="matrix" className="m-0 border border-border/50 rounded-xl overflow-hidden bg-card">
          <div className="p-4 border-b border-border/50 flex items-center justify-between bg-muted/20">
            <div className="relative w-64">
              <Search className="absolute left-2 top-2.5 h-3 w-3 text-muted-foreground" />
              <Input 
                placeholder="Filter physical models..." 
                className="pl-8 h-8 text-[10px] bg-background border-border/50" 
                value={search}
                onChange={e => setSearch(e.target.value)}
              />
            </div>
            <p className="text-[10px] font-bold text-muted-foreground uppercase tracking-widest">
              {nodes.length} Nodes · {physicalModelNames.length} Physical Models
            </p>
          </div>
          
          <div className="overflow-x-auto">
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent border-border/50">
                  <TableHead className="w-[200px] text-[10px] font-black uppercase tracking-widest py-4 pl-6">Model Identifier</TableHead>
                  {nodes.map(n => (
                    <TableHead key={n.id} className="text-center min-w-[120px] py-4">
                      <div className="flex flex-col items-center gap-1.5">
                        <span className="text-[10px] font-black uppercase tracking-tight truncate max-w-[100px]">{n.id}</span>
                        <div className="flex items-center gap-2">
                           <div className="flex items-center gap-0.5 text-[9px] font-black text-amber-400">
                             <TrendingUp size={10} />
                             {(n.reputation || 1.0).toFixed(1)}
                           </div>
                           <Badge variant="outline" className={`text-[8px] font-bold h-3.5 px-1 border-border/30 ${n.vram_total > 0 && ((n.vram_used || 0) / n.vram_total) > 0.8 ? 'text-red-400' : 'opacity-60'}`}>
                             {n.vram_total > 0 ? `${(((n.vram_used || 0) / n.vram_total) * 100).toFixed(0)}%` : 'CPU'}
                           </Badge>
                        </div>
                      </div>
                    </TableHead>
                  ))}
                </TableRow>
              </TableHeader>
              <TableBody>
                {filteredPhysicalModels.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={nodes.length + 1} className="h-32 text-center text-muted-foreground text-xs font-bold italic">
                      No physical models discovered in fleet
                    </TableCell>
                  </TableRow>
                ) : filteredPhysicalModels.map(model => (
                  <TableRow key={model} className="border-border/40 hover:bg-muted/5 group">
                    <TableCell className="font-mono text-[11px] font-black pl-6 py-4">
                      <div className="flex flex-col gap-1">
                        <div className="flex items-center gap-2">
                          <Box size={12} className="text-muted-foreground" />
                          {model}
                        </div>
                        <div className="flex items-center gap-2 ml-5">
                           <Badge variant="outline" className="text-[7px] h-3 px-1 border-amber-500/20 text-amber-500 uppercase">
                             R: {(status?.model_reward_factors?.[model] || 1.0).toFixed(1)}x
                           </Badge>
                           <Badge variant="outline" className="text-[7px] h-3 px-1 border-blue-500/20 text-blue-400 uppercase">
                             C: {(status?.model_cost_factors?.[model] || 1.0).toFixed(1)}x
                           </Badge>
                        </div>
                      </div>
                    </TableCell>
                    {nodes.map(n => {
                      const st = getModelStatusOnNode(model, n);
                      return (
                        <TableCell key={n.id} className="p-2">
                          <div className="flex flex-col items-center gap-2">
                            {/* Status Indicator */}
                            <div className="flex items-center gap-1">
                              {st.isBanned ? (
                                <Tooltip>
                                  <TooltipTrigger><ShieldX size={14} className="text-red-500/60" /></TooltipTrigger>
                                  <TooltipContent>Banned from this node</TooltipContent>
                                </Tooltip>
                              ) : st.isLoaded ? (
                                <Badge className="h-4 text-[8px] font-black bg-emerald-500/20 text-emerald-400 border-emerald-500/30">HOT</Badge>
                              ) : st.isOnDisk ? (
                                <Badge variant="outline" className="h-4 text-[8px] font-black border-blue-500/30 text-blue-400">WARM</Badge>
                              ) : (
                                <Badge variant="outline" className="h-4 text-[8px] font-black opacity-20">MISSING</Badge>
                              )}
                              {st.isPinned && <Pin size={10} className="text-amber-400 fill-amber-400/20" />}
                              {st.isPersistent && <Zap size={10} className="text-pink-400 fill-pink-400/20" />}
                              </div>

                              {/* Node Actions */}
                              <div className="opacity-0 group-hover:opacity-100 transition-opacity flex items-center gap-1">
                               {!st.isOnDisk && !st.isBanned && (
                                 <Button variant="ghost" size="icon" className="h-6 w-6 rounded-md hover:bg-blue-500/10 hover:text-blue-400" onClick={() => handlePull(model, n.id)}>
                                   <Download size={12} />
                                 </Button>
                               )}
                               {st.isOnDisk && (
                                 <Button variant="ghost" size="icon" className="h-6 w-6 rounded-md hover:bg-red-500/10 hover:text-red-400" onClick={() => handleDelete(model, n.id)}>
                                   <Trash2 size={12} />
                                 </Button>
                               )}
                               <Button
                                variant="ghost" size="icon"
                                className={`h-6 w-6 rounded-md ${st.isBanned ? 'text-red-500 bg-red-500/10' : 'text-muted-foreground'}`}
                                onClick={() => togglePolicy(model, n.id, 'banned')}
                               >
                                 <ShieldX size={12} />
                               </Button>
                               <Button
                                variant="ghost" size="icon"
                                className={`h-6 w-6 rounded-md ${st.isPinned ? 'text-amber-500 bg-amber-500/10' : 'text-muted-foreground'}`}
                                onClick={() => togglePolicy(model, n.id, 'pinned')}
                               >
                                 <Pin size={12} />
                               </Button>
                               <Button
                                variant="ghost" size="icon"
                                className={`h-6 w-6 rounded-md ${st.isPersistent ? 'text-pink-500 bg-pink-500/10' : 'text-muted-foreground'}`}
                                onClick={() => togglePolicy(model, n.id, 'persistent')}
                               >
                                 <Zap size={12} />
                               </Button>
                             </div>
                          </div>
                        </TableCell>
                      );
                    })}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </TabsContent>

        {/* ── Tab: Virtual Models ── */}
        <TabsContent value="virtual" className="m-0 space-y-4">
          <div className="flex items-center justify-between">
            <div className="relative w-64">
              <Search className="absolute left-2 top-2.5 h-3 w-3 text-muted-foreground" />
              <Input 
                placeholder="Filter virtual models..." 
                className="pl-8 h-8 text-[10px] bg-background border-border/50" 
                value={search}
                onChange={e => setSearch(e.target.value)}
              />
            </div>
            <p className="text-[10px] font-bold text-muted-foreground uppercase tracking-widest">
              {virtualModelNames.length} Virtual Configurations
            </p>
          </div>

          <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
            {filteredVirtualModels.map(name => {
              const cfg = virtualModels[name];
              return (
                <Card key={name} className="bg-card border-border/50 overflow-hidden group hover:border-primary/30 transition-colors">
                  <div className={`h-1 w-full ${cfg.type === 'pipeline' ? 'bg-purple-500' : cfg.type === 'metric' ? 'bg-blue-500' : 'bg-amber-500'}`} />
                  <CardContent className="p-5">
                    <div className="flex items-start justify-between mb-4">
                      <div className="flex items-center gap-3">
                        <div className={`p-2 rounded-lg ${cfg.type === 'pipeline' ? 'bg-purple-500/10 text-purple-400' : cfg.type === 'metric' ? 'bg-blue-500/10 text-blue-400' : 'bg-amber-500/10 text-amber-400'}`}>
                          {cfg.type === 'pipeline' ? <RefreshCw size={18} /> : <Zap size={18} />}
                        </div>
                        <div>
                          <div className="flex items-center gap-2">
                             <h3 className="text-sm font-black tracking-tight">{name}</h3>
                             <Badge variant="outline" className="text-[8px] h-3.5 px-1 uppercase font-black">{cfg.type}</Badge>
                          </div>
                          <p className="text-[10px] text-muted-foreground font-bold mt-0.5 uppercase tracking-tighter">
                            {cfg.type === 'metric' ? `Strategy: ${cfg.strategy}` : cfg.type === 'pipeline' ? `${cfg.steps?.length || 0} Steps Pipeline` : 'Arena'}
                          </p>
                        </div>
                      </div>
                      <div className="flex items-center gap-2">
                         <Badge variant="outline" className="text-[7px] h-3.5 px-1 border-amber-500/20 text-amber-500 uppercase">
                           R: {(status?.model_reward_factors?.[name] || 1.0).toFixed(1)}x
                         </Badge>
                         <Badge variant="outline" className="text-[7px] h-3.5 px-1 border-blue-500/20 text-blue-400 uppercase">
                           C: {(status?.model_cost_factors?.[name] || 1.0).toFixed(1)}x
                         </Badge>
                      </div>
                    </div>

                    <div className="space-y-3">
                      <div>
                        <p className="text-[9px] font-black uppercase text-muted-foreground mb-1.5 tracking-widest">Target Models</p>
                        <div className="flex flex-wrap gap-1.5">
                          {cfg.targets?.map(t => (
                            <Badge key={t} variant="secondary" className="text-[10px] font-mono h-5 px-1.5 bg-muted/50 border-border/50">
                              {t}
                            </Badge>
                          ))}
                        </div>
                      </div>

                      {cfg.type === 'pipeline' && cfg.steps && (
                        <div>
                          <p className="text-[9px] font-black uppercase text-muted-foreground mb-1.5 tracking-widest">Flow</p>
                          <div className="space-y-1.5">
                            {cfg.steps.map((s, idx) => (
                              <div key={idx} className="flex items-center gap-2 text-[10px] font-bold">
                                <div className="w-4 h-4 rounded-full bg-muted flex items-center justify-center text-[8px] font-black">{idx + 1}</div>
                                <span className="text-muted-foreground uppercase">{s.action}:</span>
                                <span className="font-mono">{s.model}</span>
                                {idx < cfg.steps!.length - 1 && <ChevronRight size={10} className="text-muted-foreground/30" />}
                              </div>
                            ))}
                          </div>
                        </div>
                      )}
                    </div>
                  </CardContent>
                </Card>
              );
            })}
            {filteredVirtualModels.length === 0 && (
               <div className="col-span-full h-40 flex flex-col items-center justify-center bg-muted/10 border border-dashed border-border rounded-xl opacity-50">
                <ShieldX size={32} className="mb-2 text-muted-foreground" />
                <p className="text-[10px] font-black uppercase tracking-widest text-muted-foreground">No virtual models configured</p>
              </div>
            )}
          </div>
        </TabsContent>

        {/* ── Tab: Browser ── */}
        <TabsContent value="browser" className="m-0">
          <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
            {POPULAR_MODELS.map(model => (
              <Card key={model.name} className="bg-card border-border/50 group">
                <CardContent className="p-5">
                  <div className="flex items-start justify-between mb-3">
                    <div className="flex items-center gap-3">
                      <div className="p-2 rounded-lg bg-muted/50 group-hover:bg-primary/10 transition-colors">
                        <Box size={18} />
                      </div>
                      <div>
                        <p className="text-sm font-black tracking-tight">{model.name}</p>
                        <p className="text-[10px] text-muted-foreground font-bold">{model.size}</p>
                      </div>
                    </div>
                    <div className="flex items-center gap-2">
                      <Button 
                        size="sm" 
                        variant="secondary"
                        className="h-7 text-[9px] font-black uppercase"
                        onClick={() => handlePull(model.name)}
                      >
                        Deploy Cluster
                      </Button>
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button variant="outline" size="sm" className="h-7 px-2">
                             <ChevronRight size={12} className="rotate-90" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end" className="w-48">
                          <DropdownMenuLabel className="text-[10px] font-black uppercase opacity-50">Target Node</DropdownMenuLabel>
                          {nodes.map(n => (
                            <DropdownMenuItem key={n.id} className="text-[11px] font-bold cursor-pointer" onClick={() => handlePull(model.name, n.id)}>
                              {n.id}
                            </DropdownMenuItem>
                          ))}
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </div>
                  </div>
                  <p className="text-[11px] text-muted-foreground leading-relaxed line-clamp-2">{model.desc}</p>
                </CardContent>
              </Card>
            ))}
          </div>
        </TabsContent>

        {/* ── Tab: Requests ── */}
        <TabsContent value="requests" className="m-0 space-y-3">
          {requests.length === 0 ? (
            <div className="h-40 flex flex-col items-center justify-center bg-muted/10 border border-dashed border-border rounded-xl opacity-50">
               <CheckCircle2 size={32} className="mb-2 text-emerald-500" />
               <p className="text-[10px] font-black uppercase tracking-widest text-muted-foreground">No pending actions</p>
            </div>
          ) : requests.map(req => (
            <Card key={req.id} className="bg-card border-border/50">
              <CardContent className="p-4 flex items-center gap-4">
                <div className={`p-2 rounded-lg ${req.type === 'pull' ? 'bg-blue-500/10 text-blue-400' : 'bg-red-500/10 text-red-400'}`}>
                  {req.type === 'pull' ? <Download size={18} /> : <Trash2 size={18} />}
                </div>
                <div className="flex-1">
                   <p className="text-[10px] font-black uppercase text-muted-foreground">{req.type} request</p>
                   <p className="text-xs font-black font-mono">{req.model}</p>
                   <p className="text-[9px] text-muted-foreground mt-0.5">Target: {req.node_id || 'Cluster-wide'}</p>
                </div>
                <div className="flex gap-2">
                  <Button variant="outline" size="sm" className="h-8 text-[9px] font-black uppercase" onClick={() => handleDecline(req.id)}>Decline</Button>
                  <Button size="sm" className="h-8 text-[9px] font-black uppercase" onClick={() => handleApprove(req.id)}>Approve</Button>
                </div>
              </CardContent>
            </Card>
          ))}
        </TabsContent>
      </Tabs>
    </div>
  );
};

const Layers = ({ size, className }: { size?: number, className?: string }) => (
  <svg 
    xmlns="http://www.w3.org/2000/svg" 
    width={size || 24} 
    height={size || 24} 
    viewBox="0 0 24 24" 
    fill="none" 
    stroke="currentColor" 
    strokeWidth="2" 
    strokeLinecap="round" 
    strokeLinejoin="round" 
    className={className}
  >
    <path d="m12.83 2.18a2 2 0 0 0-1.66 0L2.1 6.27a2 2 0 0 0 0 3.66l9.07 4.09a2 2 0 0 0 1.66 0l9.07-4.09a2 2 0 0 0 0-3.66z" />
    <path d="m2.1 14.15 9.07 4.09a2 2 0 0 0 1.66 0l9.07-4.09" />
    <path d="m2.1 19.15 9.07 4.09a2 2 0 0 0 1.66 0l9.07-4.09" />
  </svg>
);
