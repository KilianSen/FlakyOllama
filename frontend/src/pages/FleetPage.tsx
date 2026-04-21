import React, { useState } from 'react';
import { Zap, Cpu, MoreHorizontal, ChevronRight, TrendingUp } from 'lucide-react';
import { Card } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Progress } from '@/components/ui/progress';
import { Button } from '@/components/ui/button';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetDescription } from '@/components/ui/sheet';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Separator } from '@/components/ui/separator';
import { toast } from 'sonner';
import sdk from '../api';
import type { NodeStatus } from '../api';
import { useCluster } from '../ClusterContext';
import { inferCapabilities, CAPABILITY_LABELS, formatBytes as utilsFormatBytes } from '../lib/modelUtils';

function formatBytes(b: number) {
  if (!b) return '—';
  const gb = b / 1e9;
  return gb >= 1 ? `${gb.toFixed(1)} GB` : `${(b / 1e6).toFixed(0)} MB`;
}

function NodeStateBadge({ node }: { node: NodeStatus }) {
  if (node.state === 0 && !node.draining)
    return <Badge className="text-[9px] font-black h-5 px-2 bg-emerald-500/15 text-emerald-400 border border-emerald-500/30 uppercase">Ready</Badge>;
  if (node.state === 1)
    return <Badge className="text-[9px] font-black h-5 px-2 bg-amber-500/15 text-amber-400 border border-amber-500/30 uppercase">Degraded</Badge>;
  if (node.state === 2)
    return <Badge className="text-[9px] font-black h-5 px-2 bg-red-500/15 text-red-400 border border-red-500/30 uppercase">Offline</Badge>;
  if (node.draining)
    return <Badge className="text-[9px] font-black h-5 px-2 bg-amber-500/20 text-amber-400 border border-amber-500/40 uppercase animate-pulse">Draining</Badge>;
  return null;
}

export const FleetPage: React.FC = () => {
  const { status, refresh: onRefresh } = useCluster();
  if (!status) return null;
  const [selectedNode, setSelectedNode] = useState<NodeStatus | null>(null);
  const [deployTarget, setDeployTarget] = useState<string | null>(null);
  const nodes = Object.values(status.nodes) as NodeStatus[];

  const handleAction = async (action: () => Promise<any>, msg: string) => {
    const tid = toast.loading('Processing...');
    try {
      const res = await action();
      if (res?.job_id) await sdk.waitForJob(res.job_id);
      toast.success(msg, { id: tid });
      onRefresh();
    } catch (err: any) {
      toast.error(err.message || 'Action failed', { id: tid });
    }
  };

  return (
    <>
      <div className="p-6 space-y-4">
        <div className="flex items-center justify-between">
          <div>
            <h2 className="text-sm font-black uppercase tracking-widest">Infrastructure Fleet</h2>
            <p className="text-[11px] text-muted-foreground mt-0.5">{nodes.length} registered nodes</p>
          </div>
        </div>

        <Card className="bg-card border-border/50 overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow className="border-border/50 hover:bg-transparent">
                <TableHead className="text-[9px] font-black uppercase tracking-widest text-muted-foreground">Node</TableHead>
                <TableHead className="text-[9px] font-black uppercase tracking-widest text-muted-foreground">Tier / Hardware</TableHead>
                <TableHead className="text-[9px] font-black uppercase tracking-widest text-muted-foreground">CPU</TableHead>
                <TableHead className="text-[9px] font-black uppercase tracking-widest text-muted-foreground">VRAM / RAM</TableHead>
                <TableHead className="text-[9px] font-black uppercase tracking-widest text-muted-foreground">Reputation</TableHead>
                <TableHead className="text-[9px] font-black uppercase tracking-widest text-muted-foreground">Models</TableHead>
                <TableHead className="text-[9px] font-black uppercase tracking-widest text-muted-foreground">Status</TableHead>
                <TableHead className="text-right text-[9px] font-black uppercase tracking-widest text-muted-foreground">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {nodes.map(node => (
                <TableRow
                  key={node.id}
                  className="border-border/30 hover:bg-muted/10 cursor-pointer transition-colors"
                  onClick={() => setSelectedNode(node)}
                >
                  <TableCell className="py-4">
                    <div className="flex items-center gap-3">
                      <div className={`p-1.5 rounded-md ${node.has_gpu ? 'bg-purple-500/15 text-purple-400' : 'bg-muted text-muted-foreground'}`}>
                        {node.has_gpu ? <Zap size={14} /> : <Cpu size={14} />}
                      </div>
                      <div>
                        <p className="text-xs font-black tracking-tight">{node.id}</p>
                        <p className="text-[9px] text-muted-foreground font-mono">{node.address}</p>
                      </div>
                    </div>
                  </TableCell>
                  <TableCell>
                    <div>
                      <Badge variant="outline" className="text-[9px] font-bold h-4 mb-1 uppercase tracking-tighter">{node.tier}</Badge>
                      <p className="text-[9px] text-muted-foreground truncate max-w-[140px]">
                        {node.has_gpu ? (node.gpu_model || 'Unknown GPU') : 'CPU ONLY'}
                      </p>
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className="w-28 space-y-1">
                      <div className="flex justify-between text-[9px] font-black text-muted-foreground">
                        <span>{node.cpu_cores}c</span><span>{node.cpu_usage.toFixed(0)}%</span>
                      </div>
                      <Progress value={node.cpu_usage} className="h-1" />
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className="w-28 space-y-1">
                      {node.has_gpu ? (
                        <>
                          <div className="flex justify-between text-[9px] font-black text-muted-foreground">
                            <span>VRAM</span><span>{formatBytes(node.vram_used)}/{formatBytes(node.vram_total)}</span>
                          </div>
                          <Progress value={node.vram_total > 0 ? (node.vram_used / node.vram_total) * 100 : 0} className="h-1" />
                        </>
                      ) : (
                        <>
                          <div className="flex justify-between text-[9px] font-black text-muted-foreground">
                            <span>MEM</span><span>{node.memory_usage.toFixed(0)}%</span>
                          </div>
                          <Progress value={node.memory_usage} className="h-1" />
                        </>
                      )}
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-1">
                       <TrendingUp size={12} className={(node.reputation || 1.0) >= 1.0 ? 'text-emerald-400' : 'text-amber-400'} />
                       <span className="text-[11px] font-black">{(node.reputation || 1.0).toFixed(2)}</span>
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-1 max-w-[160px]">
                      {node.active_models?.slice(0, 2).map(m => (
                        <Badge key={m} className="text-[8px] h-4 px-1 font-black bg-primary/15 text-primary border-primary/20 truncate max-w-[80px]">{m.split(':')[0]}</Badge>
                      ))}
                      {(node.active_models?.length || 0) > 2 && (
                        <Badge variant="outline" className="text-[8px] h-4 px-1">+{(node.active_models?.length || 0) - 2}</Badge>
                      )}
                    </div>
                  </TableCell>
                  <TableCell onClick={e => e.stopPropagation()}>
                    <NodeStateBadge node={node} />
                  </TableCell>
                  <TableCell className="text-right" onClick={e => e.stopPropagation()}>
                    <div className="flex items-center justify-end gap-1">
                      <Button variant="ghost" size="icon" className="h-7 w-7 text-muted-foreground" onClick={() => setSelectedNode(node)}>
                        <ChevronRight size={14} />
                      </Button>
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button variant="ghost" size="icon" className="h-7 w-7 text-muted-foreground">
                            <MoreHorizontal size={14} />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end" className="text-xs font-bold">
                          <DropdownMenuItem onClick={() => setDeployTarget(node.id)}>Deploy Model</DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem onClick={() => handleAction(
                            () => node.draining ? sdk.undrainNode(node.id) : sdk.drainNode(node.id),
                            node.draining ? 'Node resumed' : 'Node draining'
                          )}>
                            {node.draining ? 'Resume Traffic' : 'Drain Node'}
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Card>
      </div>

      {/* Node Detail Sheet */}
      <Sheet open={!!selectedNode} onOpenChange={open => !open && setSelectedNode(null)}>
        <SheetContent className="w-[420px] sm:w-[520px] bg-card border-border/50 p-0">
          {selectedNode && (
            <>
              <SheetHeader className="p-6 border-b border-border/50">
                <div className="flex items-center gap-3">
                  <div className={`p-2 rounded-lg ${selectedNode.has_gpu ? 'bg-purple-500/15 text-purple-400' : 'bg-muted text-muted-foreground'}`}>
                    {selectedNode.has_gpu ? <Zap size={18} /> : <Cpu size={18} />}
                  </div>
                  <div>
                    <SheetTitle className="text-sm font-black uppercase tracking-tight">{selectedNode.id}</SheetTitle>
                    <SheetDescription className="text-[10px] font-mono">{selectedNode.address}</SheetDescription>
                  </div>
                  <div className="ml-auto"><NodeStateBadge node={selectedNode} /></div>
                </div>
              </SheetHeader>
              <ScrollArea className="h-[calc(100vh-120px)]">
                <div className="p-6 space-y-6">
                  {/* Status Message */}
                  {selectedNode.message && (
                    <div className={`p-4 rounded-xl border ${
                      selectedNode.state === 2 ? 'bg-red-500/5 border-red-500/20' : 
                      selectedNode.state === 1 ? 'bg-amber-500/5 border-amber-500/20' : 
                      'bg-emerald-500/5 border-emerald-500/20'
                    }`}>
                      <p className="text-[9px] font-black uppercase tracking-widest text-muted-foreground mb-1">Status Message</p>
                      <p className={`text-xs font-bold ${
                        selectedNode.state === 2 ? 'text-red-400' : 
                        selectedNode.state === 1 ? 'text-amber-400' : 
                        'text-emerald-400'
                      }`}>{selectedNode.message}</p>
                    </div>
                  )}

                  {/* Hardware */}
                  <div>
                    <p className="text-[9px] font-black uppercase tracking-widest text-muted-foreground mb-3">Hardware</p>
                    <div className="grid grid-cols-2 gap-3">
                      {[
                        { label: 'Tier', val: selectedNode.tier.toUpperCase() },
                        { label: 'CPU Cores', val: selectedNode.cpu_cores },
                        { label: 'GPU', val: selectedNode.has_gpu ? 'Yes' : 'No' },
                        { label: 'GPU Model', val: selectedNode.gpu_model || '—' },
                        { label: 'GPU Temp', val: selectedNode.gpu_temp ? `${selectedNode.gpu_temp}°C` : '—' },
                        { label: 'Total VRAM', val: formatBytes(selectedNode.vram_total) },
                        { label: 'Reputation', val: `${(selectedNode.reputation || 1.0).toFixed(2)} pts` },
                        { label: 'Errors', val: selectedNode.errors },
                      ].map(({ label, val }) => (
                        <div key={label} className="bg-muted/30 rounded-lg p-3">
                          <p className="text-[9px] font-bold text-muted-foreground uppercase tracking-wider">{label}</p>
                          <p className="text-xs font-black mt-0.5 truncate">{val}</p>
                        </div>
                      ))}
                    </div>
                  </div>

                  {/* Throughput */}
                  <div>
                    <p className="text-[9px] font-black uppercase tracking-widest text-muted-foreground mb-3">Total Throughput</p>
                    <div className="grid grid-cols-2 gap-3">
                      <div className="p-3 rounded-lg bg-emerald-500/5 border border-emerald-500/10">
                         <p className="text-[8px] font-black uppercase tracking-widest text-muted-foreground mb-1">Input Tokens</p>
                         <p className="text-xs font-black text-emerald-400">{selectedNode.input_tokens?.toLocaleString() || 0}</p>
                      </div>
                      <div className="p-3 rounded-lg bg-blue-500/5 border border-blue-500/10">
                         <p className="text-[8px] font-black uppercase tracking-widest text-muted-foreground mb-1">Output Tokens</p>
                         <p className="text-xs font-black text-blue-400">{selectedNode.output_tokens?.toLocaleString() || 0}</p>
                      </div>
                      <div className="p-3 rounded-lg bg-amber-500/5 border border-amber-500/10 col-span-2">
                         <p className="text-[8px] font-black uppercase tracking-widest text-muted-foreground mb-1">Earned Credits</p>
                         <p className="text-xs font-black text-amber-400">{selectedNode.token_reward?.toLocaleString(undefined, { minimumFractionDigits: 1, maximumFractionDigits: 1 }) || 0} φ</p>
                      </div>
                    </div>
                  </div>

                  <Separator className="bg-border/50" />
                  {/* Resources */}
                  <div>
                    <p className="text-[9px] font-black uppercase tracking-widest text-muted-foreground mb-3">Live Resources</p>
                    <div className="space-y-3">
                      <div>
                        <div className="flex justify-between text-[10px] font-black mb-1.5">
                          <span className="text-muted-foreground">CPU Load</span>
                          <span>{selectedNode.cpu_usage.toFixed(1)}%</span>
                        </div>
                        <Progress value={selectedNode.cpu_usage} className="h-2" />
                      </div>
                      {selectedNode.has_gpu && (
                        <div>
                          <div className="flex justify-between text-[10px] font-black mb-1.5">
                            <span className="text-muted-foreground">VRAM</span>
                            <span>{formatBytes(selectedNode.vram_used)} / {formatBytes(selectedNode.vram_total)}</span>
                          </div>
                          <Progress value={selectedNode.vram_total > 0 ? (selectedNode.vram_used / selectedNode.vram_total) * 100 : 0} className="h-2" />
                        </div>
                      )}
                      <div>
                        <div className="flex justify-between text-[10px] font-black mb-1.5">
                          <span className="text-muted-foreground">Memory</span>
                          <span>{selectedNode.memory_usage.toFixed(1)}%</span>
                        </div>
                        <Progress value={selectedNode.memory_usage} className="h-2" />
                      </div>
                    </div>
                  </div>
                  <Separator className="bg-border/50" />
                  {/* Models */}
                  <div>
                    <p className="text-[9px] font-black uppercase tracking-widest text-muted-foreground mb-3">Model Matrix</p>
                    <div className="space-y-3">
                      {selectedNode.active_models?.length || selectedNode.local_models?.length ? (
                        <>
                          {selectedNode.active_models?.map((m: string) => {
                            const caps = inferCapabilities(m);
                            return (
                              <div key={m} className="p-3 rounded-lg border border-emerald-500/20 bg-emerald-500/5">
                                <div className="flex items-center justify-between mb-2">
                                  <div className="flex items-center gap-2">
                                    <span className="text-xs font-black font-mono">🔥 {m}</span>
                                    <Badge className="text-[8px] font-black h-4 px-1 bg-emerald-500/10 text-emerald-400">HOT</Badge>
                                  </div>
                                  <Button
                                    size="sm" variant="ghost"
                                    className="h-6 text-[9px] font-black uppercase text-amber-400 hover:bg-amber-500/10 px-2"
                                    onClick={() => handleAction(
                                      () => sdk.unloadModel(m, selectedNode.id),
                                      `Evicted ${m} from VRAM`
                                    )}
                                  >
                                    Evict
                                  </Button>
                                </div>
                                <div className="flex flex-wrap gap-1">
                                  {caps.map(c => (
                                    <Badge key={c} className={`text-[8px] font-black h-4 px-1.5 border ${CAPABILITY_LABELS[c].color}`}>
                                      {CAPABILITY_LABELS[c].icon} {CAPABILITY_LABELS[c].label}
                                    </Badge>
                                  ))}
                                </div>
                              </div>
                            );
                          })}
                          {selectedNode.local_models?.filter((lm: {name:string}) => !selectedNode.active_models?.includes(lm.name)).map((lm: {name:string, size: number}) => {
                            const caps = inferCapabilities(lm.name);
                            return (
                              <div key={lm.name} className="p-3 rounded-lg border border-amber-500/20 bg-amber-500/5 opacity-80">
                                <div className="flex items-center justify-between mb-2">
                                  <div className="flex items-center gap-2">
                                    <span className="text-xs font-black font-mono">💾 {lm.name}</span>
                                    <Badge className="text-[8px] font-black h-4 px-1 bg-amber-500/10 text-amber-400">WARM</Badge>
                                  </div>
                                  <div className="text-[9px] font-bold text-muted-foreground">{utilsFormatBytes(lm.size)}</div>
                                </div>
                                <div className="flex flex-wrap gap-1">
                                  {caps.map(c => (
                                    <Badge key={c} className={`text-[8px] font-black h-4 px-1.5 border opacity-60 ${CAPABILITY_LABELS[c].color}`}>
                                      {CAPABILITY_LABELS[c].label}
                                    </Badge>
                                  ))}
                                </div>
                              </div>
                            );
                          })}
                        </>
                      ) : (
                        <p className="text-xs text-muted-foreground italic p-4 bg-muted/20 rounded-lg text-center">No models hosted on this node</p>
                      )}
                    </div>
                  </div>
                  <Separator className="bg-border/50" />
                  {/* Actions */}
                  <div>
                    <p className="text-[9px] font-black uppercase tracking-widest text-muted-foreground mb-3">Actions</p>
                    <div className="flex flex-wrap gap-2">
                      <Button size="sm" variant="outline" className="text-xs font-bold"
                        onClick={() => { setSelectedNode(null); setDeployTarget(selectedNode.id); }}>
                        Deploy Model
                      </Button>
                      <Button size="sm" variant="outline" className="text-xs font-bold"
                        onClick={() => handleAction(
                          () => selectedNode.draining ? sdk.undrainNode(selectedNode.id) : sdk.drainNode(selectedNode.id),
                          selectedNode.draining ? 'Node resumed' : 'Node draining'
                        )}>
                        {selectedNode.draining ? 'Resume Traffic' : 'Drain Node'}
                      </Button>
                    </div>
                  </div>
                </div>
              </ScrollArea>
            </>
          )}
        </SheetContent>
      </Sheet>

      {/* Deploy Model Dialog */}
      <Dialog open={!!deployTarget} onOpenChange={open => !open && setDeployTarget(null)}>
        <DialogContent className="bg-card border-border/50">
          <DialogHeader>
            <DialogTitle className="font-black uppercase tracking-tight">Deploy Model to Node</DialogTitle>
            <DialogDescription className="text-xs font-bold text-muted-foreground">
              Targeting: <span className="text-foreground font-mono">{deployTarget}</span>
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={e => {
            e.preventDefault();
            const tag = new FormData(e.currentTarget).get('tag') as string;
            if (deployTarget) {
              handleAction(() => sdk.pullModel(tag, deployTarget), `Deploying ${tag} to ${deployTarget}`);
              setDeployTarget(null);
            }
          }} className="space-y-4 pt-2">
            <Input name="tag" placeholder="e.g. llama3:8b" className="font-bold bg-muted/50 border-border/50" required autoFocus />
            <DialogFooter>
              <Button type="submit" className="w-full font-black uppercase text-xs tracking-widest">Initiate Pull</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </>
  );
};
