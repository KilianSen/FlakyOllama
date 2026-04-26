import React, { useState, useEffect } from 'react';
import { 
  ListOrdered, X, RefreshCw, Clock, Zap, Shield, 
  Terminal, Activity,
} from 'lucide-react';
import { toast } from 'sonner';

import { Card, CardContent, } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import sdk, { type QueuedRequest } from '@/api';

export const QueuePage: React.FC = () => {
  const [queue, setQueue] = useState<QueuedRequest[]>([]);
  const [isLoading, setIsLoading] = useState(true);

  const load = async () => {
    try {
      const data = await sdk.getQueue();
      setQueue(data || []);
    } catch (err: any) {
      toast.error('Failed to sync queue: ' + err.message);
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    load();
    const interval = setInterval(load, 3000);
    return () => clearInterval(interval);
  }, []);

  const cancelRequest = async (id: string) => {
    try {
      await sdk.cancelQueuedRequest(id);
      toast.success('Request evicted from queue');
      load();
    } catch (err: any) {
      toast.error(err.message);
    }
  };

  return (
    <div className="p-8 space-y-8 max-w-6xl mx-auto">
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4">
        <div>
          <h2 className="text-3xl font-black uppercase tracking-tighter text-foreground">Priority Queue</h2>
          <p className="text-muted-foreground font-bold tracking-tight">Real-time monitoring of pending inference workloads</p>
        </div>
        <div className="flex items-center gap-3">
          <Badge variant="outline" className="h-10 px-4 bg-primary/5 border-primary/20 flex items-center gap-2">
             <Activity size={14} className="text-primary animate-pulse" />
             <span className="text-xs font-black uppercase tracking-widest">{queue.length} Pending</span>
          </Badge>
          <Button variant="outline" size="sm" onClick={load} className="h-10 text-[10px] font-black uppercase tracking-widest gap-2">
            <RefreshCw size={14} className={isLoading ? 'animate-spin' : ''} /> Refresh
          </Button>
        </div>
      </div>

      {queue.length === 0 ? (
        <Card className="border-dashed border-border/60 bg-muted/5 py-20">
          <CardContent className="flex flex-col items-center justify-center space-y-4">
            <div className="w-16 h-16 rounded-full bg-muted flex items-center justify-center">
               <ListOrdered size={32} className="text-muted-foreground/50" />
            </div>
            <div className="text-center">
              <h3 className="text-lg font-bold uppercase tracking-tight">Queue is empty</h3>
              <p className="text-xs text-muted-foreground font-medium">No inference requests are currently waiting for a node.</p>
            </div>
          </CardContent>
        </Card>
      ) : (
        <Card className="bg-card border-border/50 shadow-2xl shadow-black/40 overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow className="hover:bg-transparent border-border/50 bg-muted/20">
                <TableHead className="text-[10px] font-black uppercase py-4 pl-6">Identifier</TableHead>
                <TableHead className="text-[10px] font-black uppercase">Model / Resource</TableHead>
                <TableHead className="text-[10px] font-black uppercase text-center">Priority</TableHead>
                <TableHead className="text-[10px] font-black uppercase text-center">Wait Time</TableHead>
                <TableHead className="text-[10px] font-black uppercase text-right pr-6">Action</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {queue.map((req) => {
                const waitMs = new Date().getTime() - new Date(req.queued_at).getTime();
                const waitSec = Math.floor(waitMs / 1000);
                
                return (
                  <TableRow key={req.id} className="border-border/40 group hover:bg-primary/5 transition-colors">
                    <TableCell className="pl-6">
                       <div className="flex flex-col gap-0.5">
                          <code className="text-[10px] font-black text-muted-foreground uppercase">{req.id.split('_').pop()}</code>
                          <div className="flex items-center gap-1.5 text-[9px] font-bold text-muted-foreground/60 uppercase">
                             <Terminal size={10} /> {req.client_ip}
                          </div>
                       </div>
                    </TableCell>
                    <TableCell>
                       <div className="flex items-center gap-2">
                          <Zap size={14} className="text-amber-400" />
                          <span className="text-xs font-black uppercase tracking-tight">{req.model}</span>
                       </div>
                    </TableCell>
                    <TableCell className="text-center">
                       <Badge className={`${req.priority > 50 ? 'bg-amber-500/10 text-amber-500' : 'bg-blue-500/10 text-blue-400'} border-none text-[10px] font-black`}>
                          P{req.priority}
                       </Badge>
                    </TableCell>
                    <TableCell className="text-center font-mono text-xs font-black">
                       <div className="flex flex-col items-center gap-1">
                          <span className={waitSec > 30 ? 'text-destructive' : 'text-foreground'}>{waitSec}s</span>
                          <div className="w-16 h-1 bg-muted rounded-full overflow-hidden">
                             <div 
                               className={`h-full ${waitSec > 30 ? 'bg-destructive' : 'bg-primary'} transition-all`} 
                               style={{ width: `${Math.min(100, (waitSec / 60) * 100)}%` }} 
                             />
                          </div>
                       </div>
                    </TableCell>
                    <TableCell className="text-right pr-6">
                       <Button 
                         variant="ghost" size="icon" 
                         className="h-8 w-8 text-muted-foreground hover:text-destructive opacity-0 group-hover:opacity-100 transition-opacity"
                         onClick={() => cancelRequest(req.id)}
                       >
                         <X size={16} />
                       </Button>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </Card>
      )}

      {/* Stats Summary */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
        <Card className="bg-card/50 border-border/40">
           <CardContent className="p-4 flex items-center gap-4">
              <div className="w-10 h-10 rounded-xl bg-blue-500/10 flex items-center justify-center">
                 <Shield size={20} className="text-blue-400" />
              </div>
              <div>
                 <p className="text-[10px] font-black uppercase text-muted-foreground tracking-widest">Enforcement</p>
                 <p className="text-xs font-black">Priority Weighted</p>
              </div>
           </CardContent>
        </Card>
        <Card className="bg-card/50 border-border/40">
           <CardContent className="p-4 flex items-center gap-4">
              <div className="w-10 h-10 rounded-xl bg-amber-500/10 flex items-center justify-center">
                 <Clock size={20} className="text-amber-400" />
              </div>
              <div>
                 <p className="text-[10px] font-black uppercase text-muted-foreground tracking-widest">Max Wait</p>
                 <p className="text-xs font-black">{queue.length > 0 ? Math.max(...queue.map(q => Math.floor((new Date().getTime() - new Date(q.queued_at).getTime()) / 1000))) : 0}s</p>
              </div>
           </CardContent>
        </Card>
        <Card className="bg-card/50 border-border/40">
           <CardContent className="p-4 flex items-center gap-4">
              <div className="w-10 h-10 rounded-xl bg-primary/10 flex items-center justify-center">
                 <Activity size={20} className="text-primary" />
              </div>
              <div>
                 <p className="text-[10px] font-black uppercase text-muted-foreground tracking-widest">Throughput</p>
                 <p className="text-xs font-black">Adaptive Scaling</p>
              </div>
           </CardContent>
        </Card>
      </div>
    </div>
  );
};

export default QueuePage;
