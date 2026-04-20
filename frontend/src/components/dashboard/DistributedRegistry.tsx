import React, { useState, useMemo } from 'react';
import { Database, Search, CloudDownload, RefreshCw, XCircle, Trash2 } from 'lucide-react';
import { Card, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger, DialogFooter } from "@/components/ui/dialog";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { toast } from 'sonner';
import sdk from '../../api';
import type { ClusterStatus } from '../../api';

interface DistributedRegistryProps {
  status: ClusterStatus;
  onRefresh: () => void;
}

export const DistributedRegistry: React.FC<DistributedRegistryProps> = ({ status, onRefresh }) => {
  const [searchModel, setSearchModel] = useState("");
  const [pullDialogOpen, setPullDialogOpen] = useState(false);

  const modelDistribution = useMemo(() => {
    return (status.all_models || []).map(m => {
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

  const handleAction = async (action: () => Promise<{ job_id?: string; status?: string }>, msg: string) => {
    const tid = toast.loading('Orchestrating...');
    try {
      const res = await action();
      if (res.job_id) {
        toast.info(`Job ${res.job_id} started`, { id: tid });
        await sdk.waitForJob(res.job_id);
        toast.success(msg, { id: tid });
      } else {
        toast.success(msg, { id: tid });
      }
      onRefresh();
    } catch (err: any) {
      toast.error(err.message || 'Action failed', { id: tid });
    }
  };

  return (
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
                <DialogTitle className="font-black tracking-tighter text-foreground uppercase">Fleet Deployment</DialogTitle>
                <DialogDescription className="text-xs font-bold uppercase text-muted-foreground">Broadcast pull command to entire compute fabric</DialogDescription>
              </DialogHeader>
              <form onSubmit={(e) => {
                e.preventDefault();
                const tag = new FormData(e.currentTarget).get('tag') as string;
                setPullDialogOpen(false);
                handleAction(() => sdk.pullModel(tag), `Successfully synced ${tag} to fleet`);
              }} className="space-y-4 pt-4">
                <Input name="tag" placeholder="e.g. llama3:8b" className="font-bold border-2" required />
                <DialogFooter>
                  <Button type="submit" className="w-full font-black uppercase text-xs tracking-widest py-6">Orchestrate Global Sync</Button>
                </DialogFooter>
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
                          <TooltipContent className="text-[10px] font-bold uppercase tracking-widest">
                            {n.id} - {n.isHot ? 'Hot (Resident in VRAM)' : 'Warm (On Disk)'}
                          </TooltipContent>
                        </Tooltip>
                      </TooltipProvider>
                    ))}
                  </div>
                </TableCell>
                <TableCell className="text-right">
                  <div className="flex justify-end gap-2 opacity-0 group-hover:opacity-100 transition-opacity">
                    <Button disabled={!!isSyncing} variant="outline" size="icon" className="h-8 w-8 text-amber-600 hover:bg-amber-50" onClick={() => handleAction(() => sdk.unloadModel(m.name), `Global evict: ${m.name}`)}><XCircle size={14} /></Button>
                    <Button disabled={!!isSyncing} variant="outline" size="icon" className="h-8 w-8 text-destructive hover:bg-destructive hover:text-white" onClick={() => handleAction(() => sdk.deleteModel(m.name), `Global purge: ${m.name}`)}><Trash2 size={14} /></Button>
                  </div>
                </TableCell>
              </TableRow>
            );
          })}
        </TableBody>
      </Table>
    </Card>
  );
};
