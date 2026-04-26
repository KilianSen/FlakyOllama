import React, { useState, useEffect } from 'react';
import { Plus, User, Copy, Zap, Server, Check, X, Clock, Trash2 } from 'lucide-react';
import { Card } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger, DialogFooter, DialogDescription } from '@/components/ui/dialog';
import { Label } from '@/components/ui/label';
import { toast } from 'sonner';
import sdk, { type ClientKey, type AgentKey } from '../api';

export const KeysPage: React.FC = () => {
  const [clients, setClients] = useState<ClientKey[]>([]);
  const [agents, setAgents] = useState<AgentKey[]>([]);
  const [loading, setLoading] = useState(true);

  const load = async () => {
    setLoading(true);
    try {
      const [c, a] = await Promise.all([sdk.getClientKeys(), sdk.getAgentKeys()]);
      setClients(c || []);
      setAgents(a || []);
    } catch (err) {
      toast.error('Failed to load cluster keys');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { load(); }, []);

  const copy = (text: string) => {
    navigator.clipboard.writeText(text);
    toast.success('Key copied to clipboard');
  };

  const setStatus = async (type: 'client' | 'agent', key: string, status: string) => {
    try {
      await sdk.setKeyStatus(type, key, status);
      toast.success(`Key ${status}`);
      load();
    } catch (err: any) {
      toast.error(err.message);
    }
  };

  const handleDelete = async (type: 'client' | 'agent', key: string) => {
     if (!confirm(`Are you sure you want to PERMANENTLY remove this ${type} token? This cannot be undone.`)) return;
     try {
       if (type === 'client') await sdk.deleteClientKey(key);
       else await sdk.deleteAgentKey(key);
       toast.success('Token revoked and deleted');
       load();
     } catch (err: any) {
       toast.error(err.message);
     }
  };

  const getStatusBadge = (status: string, active: boolean) => {
    switch (status) {
      case 'active':
        return <Badge className="bg-emerald-500/10 text-emerald-400 border-emerald-500/20 text-[9px] font-black uppercase tracking-widest">Active</Badge>;
      case 'pending':
        return <Badge variant="outline" className="text-amber-400 border-amber-500/30 text-[9px] font-black uppercase tracking-widest gap-1"><Clock size={10}/> Pending</Badge>;
      case 'rejected':
        return <Badge variant="destructive" className="text-[9px] font-black uppercase tracking-widest">Rejected</Badge>;
      default:
        return <Badge variant="outline" className="text-[9px] font-black uppercase tracking-widest">{active ? 'Active' : 'Disabled'}</Badge>;
    }
  };

  return (
    <div className="p-8 space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-black uppercase tracking-tight text-foreground">Access Control</h2>
          <p className="text-xs font-bold uppercase tracking-widest text-muted-foreground mt-1">Manage cluster-wide service accounts and node identities</p>
        </div>
        <Button variant="outline" size="sm" className="h-8 text-[10px] font-black uppercase tracking-widest" onClick={load} disabled={loading}>
           {loading ? 'Syncing...' : 'Refresh Registry'}
        </Button>
      </div>

      <Tabs defaultValue="clients" className="space-y-4">
        <TabsList className="bg-muted/30 border border-border/50">
          <TabsTrigger value="clients" className="text-[10px] font-black uppercase tracking-widest gap-2">
            <User size={12} /> Service Accounts
          </TabsTrigger>
          <TabsTrigger value="agents" className="text-[10px] font-black uppercase tracking-widest gap-2">
            <Zap size={12} /> Agent Identities
          </TabsTrigger>
        </TabsList>

        <TabsContent value="clients" className="space-y-4">
           <div className="flex justify-between items-center bg-blue-500/5 p-4 rounded-xl border border-blue-500/10">
              <p className="text-[10px] font-bold text-muted-foreground max-w-md italic">
                Service accounts provide static API keys for external applications or non-OIDC users.
              </p>
              <CreateClientKeyDialog onSuccess={load} />
           </div>
           
           <Card className="bg-card border-border/50 overflow-hidden shadow-xl shadow-black/20">
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent border-border/50 bg-muted/20">
                    <TableHead className="text-[10px] font-black uppercase py-4 pl-6">Label / Owner</TableHead>
                    <TableHead className="text-[10px] font-black uppercase">API Key</TableHead>
                    <TableHead className="text-[10px] font-black uppercase text-center">Quota / Usage</TableHead>
                    <TableHead className="text-[10px] font-black uppercase text-center">Status</TableHead>
                    <TableHead className="text-[10px] font-black uppercase text-right pr-6">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {clients.length === 0 ? (
                    <TableRow><TableCell colSpan={5} className="text-center py-12 text-muted-foreground text-xs italic">No cluster-wide keys found</TableCell></TableRow>
                  ) : clients.map(k => (
                    <TableRow key={k.key} className="border-border/40 hover:bg-muted/5 transition-colors">
                      <TableCell className="font-bold text-xs pl-6">
                        <div className="flex flex-col">
                          <span>{k.label}</span>
                          <span className="text-[9px] text-muted-foreground font-mono">{k.user_id ? `u_${k.user_id.split('_').pop()}` : 'SYSTEM'}</span>
                        </div>
                      </TableCell>
                      <TableCell>
                         <div className="flex items-center gap-2 group">
                            <code className="text-[10px] bg-muted/50 px-1.5 py-0.5 rounded font-mono text-muted-foreground">
                              {k.key.slice(0, 12)}••••••
                            </code>
                            <button onClick={() => copy(k.key)} className="opacity-0 group-hover:opacity-100 transition-opacity">
                               <Copy size={12} className="text-muted-foreground hover:text-primary" />
                            </button>
                         </div>
                      </TableCell>
                      <TableCell className="text-center">
                         <div className="space-y-1">
                            <p className="text-[10px] font-black">{k.quota_used.toLocaleString()} / {k.quota_limit === -1 ? '∞' : k.quota_limit.toLocaleString()}</p>
                            {k.quota_limit !== -1 && (
                               <div className="w-24 mx-auto h-1 bg-muted rounded-full overflow-hidden">
                                  <div className="h-full bg-primary" style={{ width: `${Math.min(100, (k.quota_used / k.quota_limit) * 100)}%` }} />
                               </div>
                            )}
                         </div>
                      </TableCell>
                      <TableCell className="text-center">
                         {getStatusBadge(k.status || '', k.active)}
                      </TableCell>
                      <TableCell className="text-right pr-6">
                        <div className="flex justify-end gap-1">
                          {k.status === 'pending' && (
                            <>
                              <Button size="icon" variant="ghost" className="h-7 w-7 text-emerald-400" onClick={() => setStatus('client', k.key, 'active')}>
                                <Check size={14} />
                              </Button>
                              <Button size="icon" variant="ghost" className="h-7 w-7 text-red-400" onClick={() => setStatus('client', k.key, 'rejected')}>
                                <X size={14} />
                              </Button>
                            </>
                          )}
                          <Button size="icon" variant="ghost" className="h-7 w-7 text-muted-foreground hover:text-destructive" onClick={() => handleDelete('client', k.key)}>
                             <Trash2 size={14} />
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
           </Card>
        </TabsContent>

        <TabsContent value="agents" className="space-y-4">
           <div className="flex justify-between items-center bg-amber-500/5 p-4 rounded-xl border border-amber-500/10">
              <p className="text-[10px] font-bold text-muted-foreground max-w-md italic">
                Agent identities allow nodes to verify themselves with the cluster.
              </p>
              <CreateAgentKeyDialog onSuccess={load} />
           </div>

           <Card className="bg-card border-border/50 overflow-hidden shadow-xl shadow-black/20">
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent border-border/50 bg-muted/20">
                    <TableHead className="text-[10px] font-black uppercase py-4 pl-6">Node / Label</TableHead>
                    <TableHead className="text-[10px] font-black uppercase">Identity Token</TableHead>
                    <TableHead className="text-[10px] font-black uppercase text-center">Credits Earned</TableHead>
                    <TableHead className="text-[10px] font-black uppercase text-center">Status</TableHead>
                    <TableHead className="text-[10px] font-black uppercase text-right pr-6">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {agents.length === 0 ? (
                    <TableRow><TableCell colSpan={5} className="text-center py-12 text-muted-foreground text-xs italic">No agent keys registered</TableCell></TableRow>
                  ) : agents.map(k => (
                    <TableRow key={k.key} className="border-border/40 hover:bg-muted/5 transition-colors">
                      <TableCell className="font-bold text-xs pl-6">
                        <div className="flex items-center gap-2">
                          <Server size={14} className="text-amber-400" />
                          <div className="flex flex-col">
                            <span>{k.label}</span>
                            <span className="text-[9px] text-muted-foreground font-mono">{k.user_id ? `u_${k.user_id.split('_').pop()}` : 'SYSTEM'}</span>
                          </div>
                        </div>
                      </TableCell>
                      <TableCell>
                         <div className="flex items-center gap-2 group">
                            <code className="text-[10px] bg-muted/50 px-1.5 py-0.5 rounded font-mono text-muted-foreground">
                              {k.key.slice(0, 12)}••••••
                            </code>
                            <button onClick={() => copy(k.key)} className="opacity-0 group-hover:opacity-100 transition-opacity">
                               <Copy size={12} className="text-muted-foreground hover:text-primary" />
                            </button>
                         </div>
                      </TableCell>
                      <TableCell className="text-center font-mono text-xs font-black text-amber-400">
                         {k.credits_earned.toLocaleString(undefined, { minimumFractionDigits: 1 })} φ
                      </TableCell>
                      <TableCell className="text-center">
                        {getStatusBadge(k.status || '', k.active)}
                      </TableCell>
                      <TableCell className="text-right pr-6">
                        <div className="flex justify-end gap-1">
                          {k.status === 'pending' && (
                            <>
                              <Button size="icon" variant="ghost" className="h-7 w-7 text-emerald-400" onClick={() => setStatus('agent', k.key, 'active')}>
                                <Check size={14} />
                              </Button>
                              <Button size="icon" variant="ghost" className="h-7 w-7 text-red-400" onClick={() => setStatus('agent', k.key, 'rejected')}>
                                <X size={14} />
                              </Button>
                            </>
                          )}
                          <Button size="icon" variant="ghost" className="h-7 w-7 text-muted-foreground hover:text-destructive" onClick={() => handleDelete('agent', k.key)}>
                             <Trash2 size={14} />
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
           </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
};

const CreateClientKeyDialog = ({ onSuccess }: { onSuccess: () => void }) => {
  const [open, setOpen] = useState(false);
  const [label, setLabel] = useState('');
  const [quota, setQuota] = useState('-1');

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await sdk.createClientKey({ 
        label, 
        quota_limit: parseInt(quota)
      });
      toast.success('Key generated');
      setOpen(false);
      onSuccess();
    } catch (err: any) { toast.error(err.message); }
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button size="sm" className="h-8 text-[10px] font-black uppercase tracking-widest gap-2">
          <Plus size={14} /> New Service Token
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-[425px] bg-card border-border/50">
        <form onSubmit={submit}>
          <DialogHeader>
            <DialogTitle className="font-black uppercase tracking-tight">Issue Service Account</DialogTitle>
            <DialogDescription className="text-xs font-bold text-muted-foreground">Create a static API key for non-interactive applications.</DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 py-6">
            <div className="grid gap-2">
              <Label className="text-[10px] font-black uppercase text-muted-foreground tracking-widest">Label / Application Name</Label>
              <Input placeholder="Analytics Worker" value={label} onChange={e => setLabel(e.target.value)} required className="font-bold" />
            </div>
            <div className="grid gap-2">
              <Label className="text-[10px] font-black uppercase text-muted-foreground tracking-widest">Initial Quota</Label>
              <Input type="number" value={quota} onChange={e => setQuota(e.target.value)} className="font-bold" />
              <p className="text-[9px] text-muted-foreground italic font-medium">-1 for unlimited tokens</p>
            </div>
          </div>
          <DialogFooter>
            <Button type="submit" className="w-full font-black uppercase tracking-widest text-xs">Create Token</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
};

const CreateAgentKeyDialog = ({ onSuccess }: { onSuccess: () => void }) => {
  const [open, setOpen] = useState(false);
  const [label, setLabel] = useState('');
  const [nodeId, setNodeId] = useState('');

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await sdk.createAgentKey({ label, node_id: nodeId });
      toast.success('Identity generated');
      setOpen(false);
      onSuccess();
    } catch (err: any) { toast.error(err.message); }
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button size="sm" variant="secondary" className="h-8 text-[10px] font-black uppercase tracking-widest gap-2 border border-amber-500/20">
          <Plus size={14} /> New Identity
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-[425px] bg-card border-border/50">
        <form onSubmit={submit}>
          <DialogHeader>
            <DialogTitle className="font-black uppercase tracking-tight text-amber-400">Create System Identity</DialogTitle>
            <DialogDescription className="text-xs font-bold text-muted-foreground">Register a persistent node identity for hardware providers.</DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 py-6">
            <div className="grid gap-2">
              <Label className="text-[10px] font-black uppercase text-muted-foreground tracking-widest">Provider Label</Label>
              <Input placeholder="NVIDIA H100 Cluster A" value={label} onChange={e => setLabel(e.target.value)} required className="font-bold" />
            </div>
            <div className="grid gap-2">
              <Label className="text-[10px] font-black uppercase text-muted-foreground tracking-widest">Target Node ID (Optional)</Label>
              <Input placeholder="swarm-valeria-1" value={nodeId} onChange={e => setNodeId(e.target.value)} className="font-bold" />
            </div>
          </div>
          <DialogFooter>
            <Button type="submit" className="w-full font-black uppercase tracking-widest text-xs bg-amber-500 hover:bg-amber-600 text-black">Generate Token</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
};
