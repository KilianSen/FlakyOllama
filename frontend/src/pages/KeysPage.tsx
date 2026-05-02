import React, { useState, useEffect } from 'react';
import { Plus, User, Copy, Zap, Server, Check, X, Clock, Trash2, KeyRound, ShieldCheck, AlertTriangle, RefreshCw, Lock, Globe } from 'lucide-react';
import { Card } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger, DialogFooter, DialogDescription } from '@/components/ui/dialog';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { toast } from 'sonner';
import sdk, { type ClientKey, type AgentKey } from '../api';

export const KeysPage: React.FC = () => {
  const [clients, setClients] = useState<ClientKey[]>([]);
  const [agents, setAgents] = useState<AgentKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [rotatingKey, setRotatingKey] = useState<AgentKey | null>(null);
  const [rotatedKey, setRotatedKey] = useState<AgentKey | null>(null);

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

  const copy = (text: string, label = 'Key') => {
    navigator.clipboard.writeText(text);
    toast.success(`${label} copied to clipboard`);
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

  const page = (
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
                    <TableHead className="text-[10px] font-black uppercase text-center">Error Format</TableHead>
                    <TableHead className="text-[10px] font-black uppercase text-center">Status</TableHead>
                    <TableHead className="text-[10px] font-black uppercase text-right pr-6">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {clients.length === 0 ? (
                    <TableRow><TableCell colSpan={6} className="text-center py-12 text-muted-foreground text-xs italic">No cluster-wide keys found</TableCell></TableRow>
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
                        <Select
                          value={k.error_format || 'standard'}
                          onValueChange={async (val) => {
                            try {
                              await sdk.updateClientKeySettings(k.key, { error_format: val === 'standard' ? '' : val });
                              toast.success('Error format updated');
                              load();
                            } catch (err: any) { toast.error(err.message); }
                          }}
                        >
                          <SelectTrigger className="h-6 text-[9px] font-black w-28 mx-auto border-border/40 bg-muted/30">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="standard" className="text-[10px]">Standard</SelectItem>
                            <SelectItem value="openai" className="text-[10px]">OpenAI</SelectItem>
                          </SelectContent>
                        </Select>
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
              <div className="space-y-1">
                <p className="text-[10px] font-bold text-muted-foreground max-w-lg italic">
                  Each agent identity contains a token pair: the <span className="text-amber-400 not-italic">Agent Token</span> is sent by the agent to the balancer, and the <span className="text-sky-400 not-italic">Balancer Token</span> is sent by the balancer to authenticate itself to the agent.
                </p>
              </div>
              <CreateAgentKeyDialog onSuccess={load} copy={copy} />
           </div>

           <Card className="bg-card border-border/50 overflow-hidden shadow-xl shadow-black/20">
              <Table>
                <TableHeader>
                  <TableRow className="hover:bg-transparent border-border/50 bg-muted/20">
                    <TableHead className="text-[10px] font-black uppercase py-4 pl-6">Node / Label</TableHead>
                    <TableHead className="text-[10px] font-black uppercase">
                      <div className="flex items-center gap-1.5"><KeyRound size={10} className="text-amber-400" /> Agent Token</div>
                    </TableHead>
                    <TableHead className="text-[10px] font-black uppercase">
                      <div className="flex items-center gap-1.5"><ShieldCheck size={10} className="text-sky-400" /> Balancer Token</div>
                    </TableHead>
                    <TableHead className="text-[10px] font-black uppercase text-center">Credits</TableHead>
                    <TableHead className="text-[10px] font-black uppercase text-center">Visibility</TableHead>
                    <TableHead className="text-[10px] font-black uppercase text-center">Status</TableHead>
                    <TableHead className="text-[10px] font-black uppercase text-right pr-6">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {agents.length === 0 ? (
                    <TableRow><TableCell colSpan={7} className="text-center py-12 text-muted-foreground text-xs italic">No agent identities registered</TableCell></TableRow>
                  ) : agents.map(k => (
                    <TableRow key={k.key} className="border-border/40 hover:bg-muted/5 transition-colors">
                      <TableCell className="font-bold text-xs pl-6">
                        <div className="flex items-center gap-2">
                          <Server size={14} className="text-amber-400 shrink-0" />
                          <div className="flex flex-col">
                            <span>{k.label}</span>
                            <span className="text-[9px] text-muted-foreground font-mono">{k.node_id || (k.user_id ? `u_${k.user_id.split('_').pop()}` : 'Global')}</span>
                          </div>
                        </div>
                      </TableCell>
                      <TableCell>
                         <div className="flex items-center gap-2 group">
                            <code className="text-[10px] bg-amber-500/10 px-1.5 py-0.5 rounded font-mono text-amber-300">
                              {k.key.slice(0, 14)}••••
                            </code>
                            <button onClick={() => copy(k.key, 'Agent Token')} className="opacity-0 group-hover:opacity-100 transition-opacity">
                               <Copy size={11} className="text-amber-400/60 hover:text-amber-400" />
                            </button>
                         </div>
                      </TableCell>
                      <TableCell>
                         {k.balancer_token ? (
                           <div className="flex items-center gap-2 group">
                              <code className="text-[10px] bg-sky-500/10 px-1.5 py-0.5 rounded font-mono text-sky-300">
                                {k.balancer_token.slice(0, 14)}••••
                              </code>
                              <button onClick={() => copy(k.balancer_token!, 'Balancer Token')} className="opacity-0 group-hover:opacity-100 transition-opacity">
                                 <Copy size={11} className="text-sky-400/60 hover:text-sky-400" />
                              </button>
                           </div>
                         ) : (
                           <span className="text-[9px] text-muted-foreground/40 italic">—</span>
                         )}
                      </TableCell>
                      <TableCell className="text-center font-mono text-xs font-black text-emerald-400">
                         {k.credits_earned >= 1_000_000 ? `${(k.credits_earned / 1_000_000).toFixed(2)}M` : k.credits_earned >= 1_000 ? `${(k.credits_earned / 1_000).toFixed(1)}k` : Math.floor(k.credits_earned).toLocaleString()} tokens
                      </TableCell>
                      <TableCell className="text-center">
                        <Select
                          value={k.model_visibility || 'public'}
                          onValueChange={async (val) => {
                            try {
                              await sdk.updateAgentKeySettings(k.key, { model_visibility: val });
                              toast.success('Visibility updated');
                              load();
                            } catch (err: any) { toast.error(err.message); }
                          }}
                        >
                          <SelectTrigger className="h-6 text-[9px] font-black w-24 mx-auto border-border/40 bg-muted/30">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="public" className="text-[10px]">
                              <div className="flex items-center gap-1.5"><Globe size={10} className="text-emerald-400" /> Public</div>
                            </SelectItem>
                            <SelectItem value="private" className="text-[10px]">
                              <div className="flex items-center gap-1.5"><Lock size={10} className="text-red-400" /> Private</div>
                            </SelectItem>
                          </SelectContent>
                        </Select>
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
                          <Button size="icon" variant="ghost" className="h-7 w-7 text-sky-400 hover:text-sky-300" title="Rotate tokens" onClick={() => setRotatingKey(k)}>
                             <RefreshCw size={13} />
                          </Button>
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

  return (
    <>
      {page}
      {rotatingKey && (
        <RotateAgentKeyDialog
          agentKey={rotatingKey}
          onClose={() => setRotatingKey(null)}
          onSuccess={(updated) => {
            setRotatingKey(null);
            setRotatedKey(updated);
            load();
          }}
        />
      )}
      {rotatedKey && (
        <TokenRevealDialog
          agentKey={rotatedKey}
          copy={copy}
          onClose={() => setRotatedKey(null)}
          title="New Tokens Issued — Save These Now"
        />
      )}
    </>
  );
};

const CreateClientKeyDialog = ({ onSuccess }: { onSuccess: () => void }) => {
  const [open, setOpen] = useState(false);
  const [label, setLabel] = useState('');
  const [quota, setQuota] = useState('-1');
  const [errorFormat, setErrorFormat] = useState('');

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await sdk.createClientKey({
        label,
        quota_limit: parseInt(quota),
        error_format: errorFormat,
      });
      toast.success('Key generated');
      setOpen(false);
      setLabel('');
      setQuota('-1');
      setErrorFormat('');
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
            <div className="grid gap-2">
              <Label className="text-[10px] font-black uppercase text-muted-foreground tracking-widest">Error Response Format</Label>
              <Select value={errorFormat || 'standard'} onValueChange={v => setErrorFormat(v === 'standard' ? '' : v)}>
                <SelectTrigger className="font-bold">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="standard">Standard (flat JSON)</SelectItem>
                  <SelectItem value="openai">OpenAI-Compatible (nested)</SelectItem>
                </SelectContent>
              </Select>
              <p className="text-[9px] text-muted-foreground italic font-medium">Use OpenAI format for clients that require structured error objects</p>
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

const TokenRevealDialog = ({ agentKey, onClose, copy, title }: { agentKey: AgentKey; onClose: () => void; copy: (v: string, l: string) => void; title?: string }) => {
  return (
    <Dialog open onOpenChange={onClose}>
      <DialogContent className="sm:max-w-[520px] bg-card border-border/50">
        <DialogHeader>
          <DialogTitle className="font-black uppercase tracking-tight text-amber-400 flex items-center gap-2">
            <AlertTriangle size={16} /> {title ?? 'Save Your Token Pair'}
          </DialogTitle>
          <DialogDescription className="text-xs font-bold text-muted-foreground">
            These credentials will only be shown in full once. Copy them now and configure your agent before closing this dialog.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-4">
          {/* Agent Token */}
          <div className="space-y-1.5">
            <div className="flex items-center gap-2">
              <KeyRound size={12} className="text-amber-400" />
              <Label className="text-[10px] font-black uppercase text-amber-400 tracking-widest">Agent Token (AGENT_TOKEN)</Label>
            </div>
            <p className="text-[9px] text-muted-foreground italic">Set on the agent. Used by the agent to authenticate itself to the balancer.</p>
            <div className="flex items-center gap-2 bg-amber-500/5 border border-amber-500/20 rounded-lg p-3">
              <code className="flex-1 text-xs font-mono text-amber-300 break-all">{agentKey.key}</code>
              <Button size="icon" variant="ghost" className="h-7 w-7 shrink-0 text-amber-400" onClick={() => copy(agentKey.key, 'Agent Token')}>
                <Copy size={13} />
              </Button>
            </div>
          </div>

          {/* Balancer Token */}
          <div className="space-y-1.5">
            <div className="flex items-center gap-2">
              <ShieldCheck size={12} className="text-sky-400" />
              <Label className="text-[10px] font-black uppercase text-sky-400 tracking-widest">Balancer Token (BALANCER_TOKEN)</Label>
            </div>
            <p className="text-[9px] text-muted-foreground italic">Set on the agent. Used by the balancer to authenticate itself when contacting this agent.</p>
            <div className="flex items-center gap-2 bg-sky-500/5 border border-sky-500/20 rounded-lg p-3">
              <code className="flex-1 text-xs font-mono text-sky-300 break-all">{agentKey.balancer_token || '—'}</code>
              {agentKey.balancer_token && (
                <Button size="icon" variant="ghost" className="h-7 w-7 shrink-0 text-sky-400" onClick={() => copy(agentKey.balancer_token!, 'Balancer Token')}>
                  <Copy size={13} />
                </Button>
              )}
            </div>
          </div>

          {/* Quick reference */}
          <div className="bg-muted/20 border border-border/50 rounded-lg p-3 space-y-1">
            <p className="text-[9px] font-black uppercase text-muted-foreground tracking-widest mb-2">Agent .env / Environment Config</p>
            <pre className="text-[10px] font-mono text-muted-foreground whitespace-pre-wrap leading-relaxed">{`AGENT_TOKEN=${agentKey.key}\nBALANCER_TOKEN=${agentKey.balancer_token || ''}`}</pre>
            <Button
              size="sm"
              variant="ghost"
              className="h-6 text-[9px] uppercase tracking-widest mt-1 gap-1.5"
              onClick={() => copy(`AGENT_TOKEN=${agentKey.key}\nBALANCER_TOKEN=${agentKey.balancer_token || ''}`, 'Config block')}
            >
              <Copy size={10} /> Copy config block
            </Button>
          </div>
        </div>

        <DialogFooter>
          <Button onClick={onClose} className="w-full font-black uppercase tracking-widest text-xs">I've saved both tokens</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

const CreateAgentKeyDialog = ({ onSuccess, copy }: { onSuccess: () => void; copy: (v: string, l: string) => void }) => {
  const [open, setOpen] = useState(false);
  const [label, setLabel] = useState('');
  const [nodeId, setNodeId] = useState('');
  const [modelVisibility, setModelVisibility] = useState('public');
  const [createdKey, setCreatedKey] = useState<AgentKey | null>(null);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      const newKey = await sdk.createAgentKey({ label, node_id: nodeId, model_visibility: modelVisibility });
      setOpen(false);
      setLabel('');
      setNodeId('');
      setModelVisibility('public');
      setCreatedKey(newKey);
      onSuccess();
    } catch (err: any) { toast.error(err.message); }
  };

  return (
    <>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogTrigger asChild>
          <Button size="sm" variant="secondary" className="h-8 text-[10px] font-black uppercase tracking-widest gap-2 border border-amber-500/20">
            <Plus size={14} /> New Identity
          </Button>
        </DialogTrigger>
        <DialogContent className="sm:max-w-[425px] bg-card border-border/50">
          <form onSubmit={submit}>
            <DialogHeader>
              <DialogTitle className="font-black uppercase tracking-tight text-amber-400">Create Agent Identity</DialogTitle>
              <DialogDescription className="text-xs font-bold text-muted-foreground">
                Generates a token pair — one for the agent to authenticate to the balancer, one for the balancer to authenticate to the agent.
              </DialogDescription>
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
              <div className="grid gap-2">
                <Label className="text-[10px] font-black uppercase text-muted-foreground tracking-widest">Model Visibility</Label>
                <Select value={modelVisibility} onValueChange={setModelVisibility}>
                  <SelectTrigger className="font-bold">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="public">
                      <div className="flex items-center gap-2"><Globe size={12} className="text-emerald-400" /> Public — shared with all users</div>
                    </SelectItem>
                    <SelectItem value="private">
                      <div className="flex items-center gap-2"><Lock size={12} className="text-red-400" /> Private — owner only</div>
                    </SelectItem>
                  </SelectContent>
                </Select>
                <p className="text-[9px] text-muted-foreground italic font-medium">Private nodes only serve requests from the node owner's account</p>
              </div>
            </div>
            <DialogFooter>
              <Button type="submit" className="w-full font-black uppercase tracking-widest text-xs bg-amber-500 hover:bg-amber-600 text-black">Generate Token Pair</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {createdKey && (
        <TokenRevealDialog
          agentKey={createdKey}
          copy={copy}
          onClose={() => setCreatedKey(null)}
        />
      )}
    </>
  );
};

const RotateAgentKeyDialog = ({
  agentKey, onClose, onSuccess,
}: {
  agentKey: AgentKey;
  onClose: () => void;
  onSuccess: (updated: AgentKey) => void;
}) => {
  const [rotateAgent, setRotateAgent] = useState(true);
  const [rotateBalancer, setRotateBalancer] = useState(true);
  const [loading, setLoading] = useState(false);

  const submit = async () => {
    if (!rotateAgent && !rotateBalancer) {
      toast.error('Select at least one token to rotate');
      return;
    }
    setLoading(true);
    try {
      const updated = await sdk.rotateAgentKey(agentKey.key, {
        rotate_agent_token: rotateAgent,
        rotate_balancer_token: rotateBalancer,
      });
      toast.success('Tokens rotated — old credentials are now invalid');
      onSuccess(updated);
    } catch (err: any) {
      toast.error(err.message);
      setLoading(false);
    }
  };

  return (
    <Dialog open onOpenChange={onClose}>
      <DialogContent className="sm:max-w-[460px] bg-card border-border/50">
        <DialogHeader>
          <DialogTitle className="font-black uppercase tracking-tight text-sky-400 flex items-center gap-2">
            <RefreshCw size={16} /> Rotate Token Pair
          </DialogTitle>
          <DialogDescription className="text-xs font-bold text-muted-foreground">
            Rotating a token immediately invalidates the previous value. The agent must be updated with the new credentials.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3 py-4">
          <p className="text-[10px] font-black uppercase text-muted-foreground tracking-widest mb-3">
            Identity: <span className="text-foreground normal-case font-bold">{agentKey.label}</span>
          </p>

          <label className="flex items-start gap-3 cursor-pointer p-3 rounded-lg border border-amber-500/20 bg-amber-500/5 hover:bg-amber-500/10 transition-colors">
            <input type="checkbox" checked={rotateAgent} onChange={e => setRotateAgent(e.target.checked)} className="mt-0.5 accent-amber-400" />
            <div>
              <p className="text-[10px] font-black uppercase text-amber-400 tracking-widest flex items-center gap-1.5"><KeyRound size={10} /> Agent Token (AGENT_TOKEN)</p>
              <p className="text-[9px] text-muted-foreground mt-0.5">Generates a new identity key. The agent must restart with the new value.</p>
            </div>
          </label>

          <label className="flex items-start gap-3 cursor-pointer p-3 rounded-lg border border-sky-500/20 bg-sky-500/5 hover:bg-sky-500/10 transition-colors">
            <input type="checkbox" checked={rotateBalancer} onChange={e => setRotateBalancer(e.target.checked)} className="mt-0.5 accent-sky-400" />
            <div>
              <p className="text-[10px] font-black uppercase text-sky-400 tracking-widest flex items-center gap-1.5"><ShieldCheck size={10} /> Balancer Token (BALANCER_TOKEN)</p>
              <p className="text-[9px] text-muted-foreground mt-0.5">Generates a new verification secret. Takes effect immediately — the agent must be updated.</p>
            </div>
          </label>
        </div>

        <DialogFooter className="gap-2">
          <Button variant="ghost" onClick={onClose} className="font-black uppercase tracking-widest text-xs">Cancel</Button>
          <Button
            onClick={submit}
            disabled={loading || (!rotateAgent && !rotateBalancer)}
            className="font-black uppercase tracking-widest text-xs bg-sky-600 hover:bg-sky-500 text-white gap-2"
          >
            <RefreshCw size={12} className={loading ? 'animate-spin' : ''} />
            {loading ? 'Rotating...' : 'Rotate Selected'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};
