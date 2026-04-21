import React, { useState, useEffect } from 'react';
import { Plus, User, Check, Copy, Zap } from 'lucide-react';
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
      toast.error('Failed to load keys');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { load(); }, []);

  const copy = (text: string) => {
    navigator.clipboard.writeText(text);
    toast.success('Key copied to clipboard');
  };

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-sm font-black uppercase tracking-widest text-foreground">Access & Economy</h2>
          <p className="text-[11px] text-muted-foreground mt-0.5">Manage API keys, quotas, and agent reward mappings</p>
        </div>
        <Button variant="outline" size="sm" className="h-8 text-[10px] font-black uppercase" onClick={load} disabled={loading}>
           {loading ? 'Syncing...' : 'Refresh Keys'}
        </Button>
      </div>

      <Tabs defaultValue="clients" className="space-y-4">
        <TabsList className="bg-muted/30 border border-border/50">
          <TabsTrigger value="clients" className="text-[10px] font-black uppercase tracking-widest gap-2">
            <User size={12} /> Client Keys
          </TabsTrigger>
          <TabsTrigger value="agents" className="text-[10px] font-black uppercase tracking-widest gap-2">
            <Zap size={12} /> Agent Keys
          </TabsTrigger>
        </TabsList>

        <TabsContent value="clients" className="space-y-4">
           <div className="flex justify-end">
              <CreateClientKeyDialog onSuccess={load} />
           </div>
           
           <Card className="bg-card border-border/50">
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent border-border/50">
                    <TableHead className="text-[10px] font-black uppercase">Label</TableHead>
                    <TableHead className="text-[10px] font-black uppercase">API Key</TableHead>
                    <TableHead className="text-[10px] font-black uppercase text-center">Quota (Tokens)</TableHead>
                    <TableHead className="text-[10px] font-black uppercase text-center">Credit Balance</TableHead>
                    <TableHead className="text-[10px] font-black uppercase text-right">Status</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {clients.length === 0 ? (
                    <TableRow><TableCell colSpan={5} className="text-center py-12 text-muted-foreground text-xs italic">No client keys issued</TableCell></TableRow>
                  ) : clients.map(k => (
                    <TableRow key={k.key} className="border-border/40">
                      <TableCell className="font-bold text-xs">{k.label}</TableCell>
                      <TableCell>
                         <div className="flex items-center gap-2 group">
                            <code className="text-[10px] bg-muted/50 px-1.5 py-0.5 rounded font-mono text-muted-foreground">
                              {k.key.slice(0, 8)}••••••••
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
                      <TableCell className="text-center font-mono text-xs font-black text-emerald-400">
                         {k.credits.toLocaleString(undefined, { minimumFractionDigits: 1 })} φ
                      </TableCell>
                      <TableCell className="text-right">
                         <Badge className={k.active ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/20' : 'bg-red-500/10 text-red-400'}>
                           {k.active ? 'ACTIVE' : 'REVOKED'}
                         </Badge>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
           </Card>
        </TabsContent>

        <TabsContent value="agents" className="space-y-4">
           <div className="flex justify-end">
              <CreateAgentKeyDialog onSuccess={load} />
           </div>

           <Card className="bg-card border-border/50">
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent border-border/50">
                    <TableHead className="text-[10px] font-black uppercase">Owner / Label</TableHead>
                    <TableHead className="text-[10px] font-black uppercase">Identity Token</TableHead>
                    <TableHead className="text-[10px] font-black uppercase text-center">Node ID</TableHead>
                    <TableHead className="text-[10px] font-black uppercase text-center">Total Earned</TableHead>
                    <TableHead className="text-[10px] font-black uppercase text-right">Status</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {agents.length === 0 ? (
                    <TableRow><TableCell colSpan={5} className="text-center py-12 text-muted-foreground text-xs italic">No agent keys registered</TableCell></TableRow>
                  ) : agents.map(k => (
                    <TableRow key={k.key} className="border-border/40">
                      <TableCell className="font-bold text-xs">{k.label}</TableCell>
                      <TableCell>
                         <div className="flex items-center gap-2 group">
                            <code className="text-[10px] bg-muted/50 px-1.5 py-0.5 rounded font-mono text-muted-foreground">
                              {k.key.slice(0, 8)}••••••••
                            </code>
                            <button onClick={() => copy(k.key)} className="opacity-0 group-hover:opacity-100 transition-opacity">
                               <Copy size={12} className="text-muted-foreground hover:text-primary" />
                            </button>
                         </div>
                      </TableCell>
                      <TableCell className="text-center font-mono text-[10px] font-black text-muted-foreground">
                        {k.node_id || 'ANY'}
                      </TableCell>
                      <TableCell className="text-center font-mono text-xs font-black text-amber-400">
                         {k.credits_earned.toLocaleString(undefined, { minimumFractionDigits: 1 })} φ
                      </TableCell>
                      <TableCell className="text-right">
                         <Badge className={k.active ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/20' : 'bg-red-500/10 text-red-400'}>
                           {k.active ? 'VERIFIED' : 'DISABLED'}
                         </Badge>
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
  const [credits, setCredits] = useState('0');

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await sdk.createClientKey({ 
        label, 
        quota_limit: parseInt(quota), 
        credits: parseFloat(credits),
        active: true 
      });
      toast.success('Client key created');
      setOpen(false);
      onSuccess();
    } catch (err: any) { toast.error(err.message); }
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button size="sm" className="h-8 text-[10px] font-black uppercase tracking-widest gap-2">
          <Plus size={14} /> Issue Client Key
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-[425px]">
        <form onSubmit={submit}>
          <DialogHeader>
            <DialogTitle>Issue New Client Key</DialogTitle>
            <DialogDescription>Create a unique API key for an application or user.</DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 py-4">
            <div className="grid gap-2">
              <Label htmlFor="label">Label</Label>
              <Input id="label" placeholder="Production App A" value={label} onChange={e => setLabel(e.target.value)} required />
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div className="grid gap-2">
                <Label htmlFor="quota">Token Quota</Label>
                <Input id="quota" type="number" value={quota} onChange={e => setQuota(e.target.value)} />
                <p className="text-[10px] text-muted-foreground">-1 for unlimited</p>
              </div>
              <div className="grid gap-2">
                <Label htmlFor="credits">Initial Credits</Label>
                <Input id="credits" type="number" value={credits} onChange={e => setCredits(e.target.value)} />
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button type="submit">Create API Key</Button>
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
      await sdk.createAgentKey({ label, node_id: nodeId, active: true });
      toast.success('Agent identity created');
      setOpen(false);
      onSuccess();
    } catch (err: any) { toast.error(err.message); }
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button size="sm" variant="secondary" className="h-8 text-[10px] font-black uppercase tracking-widest gap-2">
          <Plus size={14} /> New Agent Identity
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-[425px]">
        <form onSubmit={submit}>
          <DialogHeader>
            <DialogTitle>Create Agent Identity</DialogTitle>
            <DialogDescription>Link compute contributions to a specific reward account.</DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 py-4">
            <div className="grid gap-2">
              <Label htmlFor="alabel">Owner Label</Label>
              <Input id="alabel" placeholder="Provider: John Doe" value={label} onChange={e => setLabel(e.target.value)} required />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="node">Target Node ID</Label>
              <Input id="node" placeholder=" swarm-valeria-1" value={nodeId} onChange={e => setNodeId(e.target.value)} />
              <p className="text-[10px] text-muted-foreground">Optional: Lock this key to a specific Node ID</p>
            </div>
          </div>
          <DialogFooter>
            <Button type="submit">Generate Token</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
};
