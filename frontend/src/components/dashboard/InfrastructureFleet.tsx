import React, { useState } from 'react';
import { Server, Zap, Cpu, MoreVertical } from 'lucide-react';
import { Card, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Progress } from "@/components/ui/progress";
import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { toast } from 'sonner';
import sdk from '../../api';
import type { ClusterStatus } from '../../api';

interface InfrastructureFleetProps {
  status: ClusterStatus;
  onRefresh: () => void;
}

export const InfrastructureFleet: React.FC<InfrastructureFleetProps> = ({ status, onRefresh }) => {
  const [nodePullTarget, setNodePullTarget] = useState<string | null>(null);

  const handleAction = async (action: () => Promise<any>, msg: string) => {
    const tid = toast.loading('Updating node state...');
    try {
      const res = await action();
      if (res.job_id) {
        await sdk.waitForJob(res.job_id);
      }
      toast.success(msg, { id: tid });
      onRefresh();
    } catch (err: any) {
      toast.error(err.message || 'Action failed', { id: tid });
    }
  };

  return (
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
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-black tracking-tight">{node.id}</span>
                      <Badge variant="outline" className="text-[8px] font-black h-4 px-1 leading-none uppercase bg-muted/50">{node.tier}</Badge>
                    </div>
                    <span className="text-[9px] font-bold text-muted-foreground font-mono">{node.address}</span>
                  </div>
                </div>
              </TableCell>
              <TableCell>
                <div className="flex flex-col gap-3 w-48">
                  <div className="space-y-1">
                    <div className="flex justify-between text-[8px] font-black text-muted-foreground uppercase tracking-widest"><span>CPU LOAD</span> <span>{node.cpu_usage.toFixed(0)}%</span></div>
                    <Progress value={node.cpu_usage} indicatorClassName="bg-blue-500" className="h-1" />
                  </div>
                  <div className="space-y-1">
                    <div className="flex justify-between text-[8px] font-black text-muted-foreground uppercase tracking-widest">
                      <span>{node.has_gpu ? `VRAM` : 'RAM'}</span>
                      <span>
                        {node.has_gpu 
                          ? `${((node.vram_total - node.vram_used) / 1e9).toFixed(1)}G FREE` 
                          : `${node.memory_usage.toFixed(0)}%`}
                      </span>
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
                      <Badge variant="destructive" className="text-[9px] font-black h-5 uppercase tracking-widest">OFFLINE</Badge>
                    )}
                    {node.draining && <Badge className="text-[9px] font-black h-5 bg-amber-500 text-white leading-none flex items-center justify-center px-2">DRAINING</Badge>}
                  </div>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild><Button variant="ghost" size="icon" className="h-7 w-7"><MoreVertical size={14} /></Button></DropdownMenuTrigger>
                    <DropdownMenuContent align="end" className="font-black text-[10px] uppercase tracking-widest text-foreground">
                      <DropdownMenuItem onClick={() => handleAction(() => node.draining ? sdk.undrainNode(node.id) : sdk.drainNode(node.id), `Node state updated`)}>
                        {node.draining ? 'RESUME TRAFFIC' : 'DRAIN NODE'}
                      </DropdownMenuItem>
                      <DropdownMenuItem onClick={() => setNodePullTarget(node.id)}>DEPLOY TO NODE</DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>

      <Dialog open={!!nodePullTarget} onOpenChange={(open) => !open && setNodePullTarget(null)}>
        <DialogContent className="rounded-2xl">
          <DialogHeader>
            <DialogTitle className="font-black tracking-tighter text-foreground uppercase">Node-Specific Deployment</DialogTitle>
            <DialogDescription className="text-xs font-bold uppercase text-muted-foreground">Targeting worker: {nodePullTarget}</DialogDescription>
          </DialogHeader>
          <form onSubmit={(e) => {
            e.preventDefault();
            const tag = new FormData(e.currentTarget).get('tag') as string;
            if (nodePullTarget) {
              handleAction(() => sdk.pullModel(tag, nodePullTarget), `Deployed ${tag} to ${nodePullTarget}`);
              setNodePullTarget(null);
            }
          }} className="space-y-4 pt-4">
            <Input name="tag" placeholder="e.g. mistral" className="font-bold border-2 h-12 rounded-xl shadow-inner" required />
            <DialogFooter>
              <Button type="submit" className="w-full font-black uppercase text-xs tracking-widest h-12 rounded-xl shadow-lg">Initiate Targeted Pull</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </Card>
  );
};
