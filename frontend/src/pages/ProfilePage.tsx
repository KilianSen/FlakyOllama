import { useState, useEffect } from 'react';
import {
  Key, User as UserIcon, Shield, Copy, Check, AlertTriangle,
  Zap, RefreshCw, Server, Plus, KeyRound, ShieldCheck, Trash2, Globe, Lock
} from 'lucide-react';
import { toast } from 'sonner';

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Progress } from '@/components/ui/progress';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog';
import sdk, { type ProfileResponse, setToken, type ClientKey, type AgentKey, type QuotaTier } from '@/api';

const TIER_LABELS: Record<QuotaTier, string> = {
  free: 'Free', standard: 'Standard', pro: 'Pro', unlimited: 'Unlimited', custom: 'Custom',
};

function quotaBar(used: number, limit: number, creditOffset: number, label: string) {
  const effective = Math.max(0, used - creditOffset);
  const pct = limit === -1 ? 0 : Math.min(100, (effective / limit) * 100);
  const over = limit !== -1 && effective >= limit;
  return (
    <div className="space-y-1">
      <div className="flex justify-between items-center">
        <span className="text-[9px] font-black uppercase tracking-widest text-muted-foreground">{label}</span>
        <span className={`text-[9px] font-black ${over ? 'text-destructive' : 'text-muted-foreground'}`}>
          {effective.toLocaleString()} / {limit === -1 ? '∞' : limit.toLocaleString()}
        </span>
      </div>
      <Progress value={pct} className={`h-1 ${over ? '[&>div]:bg-destructive' : ''}`} />
    </div>
  );
}

/* ─── helpers ─────────────────────────────────────────── */

const TokenRevealDialog = ({
  agentKey, onClose, title,
}: { agentKey: AgentKey; onClose: () => void; title?: string }) => {
  const copy = (v: string, label: string) => {
    navigator.clipboard.writeText(v);
    toast.success(`${label} copied`);
  };

  return (
    <Dialog open onOpenChange={onClose}>
      <DialogContent className="sm:max-w-[520px] bg-card border-border/50">
        <DialogHeader>
          <DialogTitle className="font-black uppercase tracking-tight text-amber-400 flex items-center gap-2">
            <AlertTriangle size={16} /> {title ?? 'Save Your Token Pair'}
          </DialogTitle>
          <DialogDescription className="text-xs font-bold text-muted-foreground">
            These credentials will only be shown in full once. Copy them now and configure your agent.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-4">
          <div className="space-y-1.5">
            <div className="flex items-center gap-2">
              <KeyRound size={12} className="text-amber-400" />
              <Label className="text-[10px] font-black uppercase text-amber-400 tracking-widest">Agent Token (AGENT_TOKEN)</Label>
            </div>
            <p className="text-[9px] text-muted-foreground italic">Used by the agent to authenticate itself to the balancer.</p>
            <div className="flex items-center gap-2 bg-amber-500/5 border border-amber-500/20 rounded-lg p-3">
              <code className="flex-1 text-xs font-mono text-amber-300 break-all">{agentKey.key}</code>
              <Button size="icon" variant="ghost" className="h-7 w-7 shrink-0 text-amber-400" onClick={() => copy(agentKey.key, 'Agent Token')}>
                <Copy size={13} />
              </Button>
            </div>
          </div>

          <div className="space-y-1.5">
            <div className="flex items-center gap-2">
              <ShieldCheck size={12} className="text-sky-400" />
              <Label className="text-[10px] font-black uppercase text-sky-400 tracking-widest">Balancer Token (BALANCER_TOKEN)</Label>
            </div>
            <p className="text-[9px] text-muted-foreground italic">Used by the balancer to authenticate itself when contacting this agent.</p>
            <div className="flex items-center gap-2 bg-sky-500/5 border border-sky-500/20 rounded-lg p-3">
              <code className="flex-1 text-xs font-mono text-sky-300 break-all">{agentKey.balancer_token || '—'}</code>
              {agentKey.balancer_token && (
                <Button size="icon" variant="ghost" className="h-7 w-7 shrink-0 text-sky-400" onClick={() => copy(agentKey.balancer_token!, 'Balancer Token')}>
                  <Copy size={13} />
                </Button>
              )}
            </div>
          </div>

          <div className="bg-muted/20 border border-border/50 rounded-lg p-3 space-y-1">
            <p className="text-[9px] font-black uppercase text-muted-foreground tracking-widest mb-2">Agent Environment Config</p>
            <pre className="text-[10px] font-mono text-muted-foreground whitespace-pre-wrap leading-relaxed">{`AGENT_TOKEN=${agentKey.key}\nBALANCER_TOKEN=${agentKey.balancer_token || ''}`}</pre>
            <Button size="sm" variant="ghost" className="h-6 text-[9px] uppercase tracking-widest mt-1 gap-1.5"
              onClick={() => copy(`AGENT_TOKEN=${agentKey.key}\nBALANCER_TOKEN=${agentKey.balancer_token || ''}`, 'Config block')}>
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

const RotateAgentKeyDialog = ({
  agentKey, onClose, onSuccess,
}: { agentKey: AgentKey; onClose: () => void; onSuccess: (updated: AgentKey) => void }) => {
  const [rotateAgent, setRotateAgent] = useState(true);
  const [rotateBalancer, setRotateBalancer] = useState(true);
  const [loading, setLoading] = useState(false);

  const submit = async () => {
    if (!rotateAgent && !rotateBalancer) { toast.error('Select at least one token'); return; }
    setLoading(true);
    try {
      const updated = await sdk.myRotateAgentKey(agentKey.key, {
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
      <DialogContent className="sm:max-w-[440px] bg-card border-border/50">
        <DialogHeader>
          <DialogTitle className="font-black uppercase tracking-tight text-sky-400 flex items-center gap-2">
            <RefreshCw size={16} /> Rotate Token Pair
          </DialogTitle>
          <DialogDescription className="text-xs font-bold text-muted-foreground">
            Rotating a token immediately invalidates the previous value.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3 py-4">
          <p className="text-[10px] font-black uppercase text-muted-foreground tracking-widest">
            Identity: <span className="text-foreground normal-case font-bold">{agentKey.label}</span>
          </p>
          <label className="flex items-start gap-3 cursor-pointer p-3 rounded-lg border border-amber-500/20 bg-amber-500/5 hover:bg-amber-500/10 transition-colors">
            <input type="checkbox" checked={rotateAgent} onChange={e => setRotateAgent(e.target.checked)} className="mt-0.5 accent-amber-400" />
            <div>
              <p className="text-[10px] font-black uppercase text-amber-400 tracking-widest flex items-center gap-1.5"><KeyRound size={10} /> Agent Token (AGENT_TOKEN)</p>
              <p className="text-[9px] text-muted-foreground mt-0.5">Generates a new identity key. Requires agent restart.</p>
            </div>
          </label>
          <label className="flex items-start gap-3 cursor-pointer p-3 rounded-lg border border-sky-500/20 bg-sky-500/5 hover:bg-sky-500/10 transition-colors">
            <input type="checkbox" checked={rotateBalancer} onChange={e => setRotateBalancer(e.target.checked)} className="mt-0.5 accent-sky-400" />
            <div>
              <p className="text-[10px] font-black uppercase text-sky-400 tracking-widest flex items-center gap-1.5"><ShieldCheck size={10} /> Balancer Token (BALANCER_TOKEN)</p>
              <p className="text-[9px] text-muted-foreground mt-0.5">Takes effect immediately — agent must be updated.</p>
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

/* ─── main page ────────────────────────────────────────── */

const ProfilePage = () => {
  const [profile, setProfile] = useState<ProfileResponse | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [copiedKey, setCopiedKey] = useState<string | null>(null);
  const [isApplying, setIsApplying] = useState(false);
  const [rotatingKey, setRotatingKey] = useState<AgentKey | null>(null);
  const [revealKey, setRevealKey] = useState<AgentKey | null>(null);
  const [revealTitle, setRevealTitle] = useState<string | undefined>(undefined);

  // New Client Key Dialog
  const [isNewKeyOpen, setIsNewKeyOpen] = useState(false);
  const [newKeyLabel, setNewKeyLabel] = useState('');
  const [newKeyQuota, setNewKeyQuota] = useState('1000000');

  const load = () => {
    setIsLoading(true);
    sdk.getMe()
      .then(res => {
        setProfile(res);
        if (res.client_keys && res.client_keys.length > 0 && !localStorage.getItem('BALANCER_TOKEN')) {
           const activeKey = res.client_keys.find(k => k.active && k.status === 'active');
           if (activeKey) setToken(activeKey.key);
        }
      })
      .catch(err => toast.error('Failed to load profile: ' + err.message))
      .finally(() => setIsLoading(false));
  };

  useEffect(() => { load(); }, []);

  const copyToClipboard = (text: string, type: string) => {
    navigator.clipboard.writeText(text);
    setCopiedKey(text);
    toast.success(`${type} copied to clipboard`);
    setTimeout(() => setCopiedKey(null), 2000);
  };

  const handleCreateClientKey = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await sdk.myCreateClientKey({ label: newKeyLabel, quota_limit: parseInt(newKeyQuota) });
      toast.success('API Key created');
      setIsNewKeyOpen(false);
      setNewKeyLabel('');
      load();
    } catch (err: any) {
      toast.error(err.message);
    }
  };

  const handleDeleteClientKey = async (key: string) => {
    if (!confirm('Permanently delete this API key? This cannot be undone.')) return;
    try {
      await sdk.myDeleteClientKey(key);
      toast.success('API key deleted');
      load();
    } catch (err: any) {
      toast.error(err.message);
    }
  };

  const handleUpdateClientKeyFormat = async (key: string, format: string) => {
    try {
      await sdk.myUpdateClientKeySettings(key, { error_format: format });
      toast.success('Error format updated');
      load();
    } catch (err: any) {
      toast.error(err.message);
    }
  };

  const handleDeleteAgentKey = async (key: string) => {
    if (!confirm('Permanently delete this agent identity? This cannot be undone.')) return;
    try {
      await sdk.myDeleteAgentKey(key);
      toast.success('Agent identity deleted');
      load();
    } catch (err: any) {
      toast.error(err.message);
    }
  };

  const handleUpdateAgentVisibility = async (key: string, visibility: string) => {
    try {
      await sdk.myUpdateAgentKeySettings(key, { model_visibility: visibility });
      toast.success('Visibility updated');
      load();
    } catch (err: any) {
      toast.error(err.message);
    }
  };

  const requestAgentKey = async () => {
    setIsApplying(true);
    try {
      const newKey = await sdk.myCreateAgentKey({ label: `Agent for ${profile?.user.name}` });
      toast.success('Agent identity generated — save your tokens!');
      setRevealTitle('Save Your New Token Pair');
      setRevealKey(newKey);
      load();
    } catch (err: any) {
      toast.error('Failed to create agent identity: ' + err.message);
    } finally {
      setIsApplying(false);
    }
  };

  if (isLoading && !profile) {
    return (
      <div className="flex items-center justify-center h-full">
        <RefreshCw className="animate-spin text-primary" size={24} />
      </div>
    );
  }

  if (!profile) return null;

  const { user, client_keys: rawKeys, agent_keys: rawAgents } = profile;
  const keys = rawKeys || [];
  const agents = rawAgents || [];
  const totalEarned = agents.reduce((sum: number, ak: AgentKey) => sum + (ak.credits_earned || 0), 0);
  const usage = profile.quota_usage ?? { daily_used: 0, weekly_used: 0, monthly_used: 0, agent_credits_earned: 0 };
  const creditOffset = Math.floor(usage.agent_credits_earned);
  const tier = (user.quota_tier || 'custom') as QuotaTier;

  return (
    <>
      <div className="p-8 max-w-5xl mx-auto space-y-8 pb-20">
        <div className="flex flex-col gap-2">
          <h2 className="text-3xl font-black uppercase tracking-tighter">My Profile</h2>
          <p className="text-muted-foreground font-bold tracking-tight">Manage your account, access keys, and compute contributions.</p>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
          {/* User Info & Global Quota */}
          <div className="md:col-span-1 space-y-6">
            <Card className="border-border/40 shadow-xl shadow-black/20 bg-card/50 backdrop-blur-md">
              <CardHeader className="pb-4">
                <div className="flex items-center gap-4">
                  <div className="w-16 h-16 rounded-2xl bg-primary/10 border border-primary/20 flex items-center justify-center">
                    <UserIcon size={32} className="text-primary" />
                  </div>
                  <div className="min-w-0">
                    <CardTitle className="text-xl font-black truncate">{user.name}</CardTitle>
                    <CardDescription className="font-bold truncate">{user.email}</CardDescription>
                  </div>
                </div>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="flex flex-col gap-1.5 p-3 rounded-xl bg-muted/30 border border-border/50">
                  <span className="text-[10px] font-black uppercase text-muted-foreground tracking-widest">Account Type</span>
                  <div className="flex items-center gap-2">
                    {user.is_admin ? (
                      <Badge className="bg-primary/20 text-primary border-primary/30 font-black uppercase text-[10px] tracking-widest px-2 py-0.5">
                        <Shield size={10} className="mr-1" /> Administrator
                      </Badge>
                    ) : (
                      <Badge variant="secondary" className="font-black uppercase text-[10px] tracking-widest px-2 py-0.5">
                        Regular User
                      </Badge>
                    )}
                  </div>
                </div>
                <div className="flex flex-col gap-1.5 p-3 rounded-xl bg-muted/30 border border-border/50">
                  <span className="text-[10px] font-black uppercase text-muted-foreground tracking-widest">Compute Usage</span>
                  <div className="flex items-center gap-1.5 text-amber-400">
                     <Zap size={14} />
                     <span className="text-sm font-black">{(user.quota_used / 100).toFixed(2)} φ spent</span>
                  </div>
                </div>
                {totalEarned > 0 && (
                  <div className="flex flex-col gap-1.5 p-3 rounded-xl bg-emerald-500/5 border border-emerald-500/20">
                    <span className="text-[10px] font-black uppercase text-muted-foreground tracking-widest">Total Earned</span>
                    <div className="flex items-center gap-1.5 text-emerald-400">
                       <Zap size={14} />
                       <span className="text-sm font-black">{totalEarned.toFixed(2)} φ</span>
                    </div>
                    {totalEarned > (user.quota_used / 100) && (
                      <span className="text-[9px] text-emerald-400/70 font-bold">Net positive contributor</span>
                    )}
                  </div>
                )}
              </CardContent>
            </Card>

            <Card className="border-border/40 bg-primary/5">
              <CardContent className="p-4 space-y-3">
                <div className="flex items-center justify-between mb-1">
                  <label className="text-[10px] font-black uppercase text-primary tracking-widest">Quota — {TIER_LABELS[tier]}</label>
                  {creditOffset > 0 && (
                    <span className="text-[9px] font-black text-emerald-400">-{creditOffset.toLocaleString()} credit offset</span>
                  )}
                </div>
                {quotaBar(usage.daily_used, user.daily_quota_limit, creditOffset, 'Daily')}
                {quotaBar(usage.weekly_used, user.weekly_quota_limit, creditOffset, 'Weekly')}
                {quotaBar(usage.monthly_used, user.monthly_quota_limit, creditOffset, 'Monthly')}
                {quotaBar(user.quota_used, user.quota_limit, creditOffset, 'Total (lifetime)')}
              </CardContent>
            </Card>
          </div>

          {/* API Keys List */}
          <div className="md:col-span-2 space-y-6">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <Key size={18} className="text-primary" />
                <h3 className="text-lg font-black uppercase tracking-tight">API Access Keys</h3>
              </div>
              <Dialog open={isNewKeyOpen} onOpenChange={setIsNewKeyOpen}>
                <DialogTrigger asChild>
                  <Button size="sm" variant="outline" className="h-8 text-[10px] font-black uppercase gap-2">
                    <Plus size={14} /> New Key
                  </Button>
                </DialogTrigger>
                <DialogContent className="bg-card border-border/50">
                  <form onSubmit={handleCreateClientKey}>
                    <DialogHeader>
                      <DialogTitle className="font-black uppercase tracking-tight">Create API Key</DialogTitle>
                      <DialogDescription className="text-xs font-bold text-muted-foreground">Issue a new token for inference with an optional sub-quota.</DialogDescription>
                    </DialogHeader>
                    <div className="grid gap-4 py-6">
                      <div className="space-y-2">
                        <Label className="text-[10px] font-black uppercase text-muted-foreground tracking-widest">Key Label</Label>
                        <Input placeholder="Personal Project A" value={newKeyLabel} onChange={e => setNewKeyLabel(e.target.value)} required />
                      </div>
                      <div className="space-y-2">
                        <Label className="text-[10px] font-black uppercase text-muted-foreground tracking-widest">Sub-Quota Limit</Label>
                        <Input type="number" value={newKeyQuota} onChange={e => setNewKeyQuota(e.target.value)} />
                        <p className="text-[9px] text-muted-foreground italic">Limits this specific key's usage. Contributes to global quota.</p>
                      </div>
                    </div>
                    <DialogFooter>
                      <Button type="submit" className="w-full font-black uppercase text-xs tracking-widest">Generate Token</Button>
                    </DialogFooter>
                  </form>
                </DialogContent>
              </Dialog>
            </div>

            <div className="grid gap-3">
              {keys.map((k: ClientKey) => {
                const keyPercent = k.quota_limit > 0 ? (k.quota_used / k.quota_limit) * 100 : 0;
                return (
                  <Card key={k.key} className="bg-card/30 border-border/40 hover:border-primary/30 transition-colors group">
                    <CardContent className="p-4 space-y-3">
                      <div className="flex flex-col sm:flex-row sm:items-center gap-4">
                        <div className="flex-1 min-w-0 space-y-1">
                          <div className="flex items-center gap-2">
                            <span className="text-xs font-black uppercase tracking-tight truncate">{k.label}</span>
                            {!k.active && <Badge variant="destructive" className="text-[8px] h-4">INACTIVE</Badge>}
                          </div>
                          <div className="flex items-center gap-2">
                            <code className="text-[10px] font-mono text-muted-foreground bg-black/20 px-1.5 py-0.5 rounded truncate max-w-[200px]">
                              {k.key}
                            </code>
                            <Button
                              size="icon" variant="ghost" className="h-6 w-6 opacity-0 group-hover:opacity-100 transition-opacity"
                              onClick={() => copyToClipboard(k.key, 'API Key')}
                            >
                              {copiedKey === k.key ? <Check size={12} className="text-emerald-400" /> : <Copy size={12} />}
                            </Button>
                          </div>
                        </div>
                        <div className="w-full sm:w-32 space-y-1">
                          <div className="flex justify-between text-[8px] font-black uppercase text-muted-foreground">
                            <span>Usage</span>
                            <span>{k.quota_limit === -1 ? '∞' : `${Math.round(keyPercent)}%`}</span>
                          </div>
                          <Progress value={k.quota_limit === -1 ? 0 : keyPercent} className="h-1" />
                        </div>
                        <Button
                          size="icon" variant="ghost"
                          className="h-7 w-7 text-muted-foreground hover:text-destructive shrink-0"
                          title="Delete key"
                          onClick={() => handleDeleteClientKey(k.key)}
                        >
                          <Trash2 size={13} />
                        </Button>
                      </div>
                      {/* Settings row */}
                      <div className="flex items-center gap-3 pt-1 border-t border-border/30">
                        <span className="text-[9px] font-black uppercase text-muted-foreground tracking-widest shrink-0">Error Format</span>
                        <Select value={k.error_format || ''} onValueChange={(val) => handleUpdateClientKeyFormat(k.key, val)}>
                          <SelectTrigger className="h-6 text-[9px] font-black w-44 border-border/40 bg-muted/30">
                            <SelectValue placeholder="Standard" />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="" className="text-[10px]">Standard (flat JSON)</SelectItem>
                            <SelectItem value="openai" className="text-[10px]">OpenAI-Compatible (nested)</SelectItem>
                          </SelectContent>
                        </Select>
                      </div>
                    </CardContent>
                  </Card>
                );
              })}
            </div>
          </div>
        </div>

        {/* Agent Identities */}
        <div className="space-y-6 pt-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <Zap className="text-amber-400" size={24} />
              <h3 className="text-xl font-black uppercase tracking-tight">Agent Identities</h3>
            </div>
            <Button 
              size="sm" 
              variant="outline" 
              className="h-8 text-[10px] font-black uppercase tracking-widest gap-2"
              onClick={requestAgentKey}
              disabled={isApplying}
            >
              {isApplying ? <RefreshCw size={12} className="animate-spin" /> : <Plus size={12} />}
              New Identity
            </Button>
          </div>
          
          {agents.length === 0 && (
            <div className="text-center py-12 text-muted-foreground text-xs italic border border-border/30 rounded-xl bg-muted/5">
              No agent identities. Click "New Identity" to generate a token pair for your compute node.
            </div>
          )}

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            {agents.map((ak: AgentKey) => (
              <Card key={ak.key} className="border-border/40 bg-card/30 backdrop-blur-sm group overflow-hidden">
                <CardHeader className="p-4 pb-2">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2">
                      <Server size={14} className="text-amber-400" />
                      <span className="text-xs font-black uppercase tracking-tight">{ak.label}</span>
                    </div>
                    <div className="flex items-center gap-1.5">
                      <Badge variant="outline" className="text-[8px] font-black border-emerald-500/30 text-emerald-400 uppercase">
                        {ak.active ? 'Active' : 'Inactive'}
                      </Badge>
                      <Button
                        size="icon" variant="ghost"
                        className="h-6 w-6 text-sky-400 hover:text-sky-300"
                        title="Rotate tokens"
                        onClick={() => setRotatingKey(ak)}
                      >
                        <RefreshCw size={12} />
                      </Button>
                      <Button
                        size="icon" variant="ghost"
                        className="h-6 w-6 text-muted-foreground hover:text-destructive"
                        title="Delete identity"
                        onClick={() => handleDeleteAgentKey(ak.key)}
                      >
                        <Trash2 size={12} />
                      </Button>
                    </div>
                  </div>
                </CardHeader>
                <CardContent className="p-4 pt-0 space-y-3">
                  {/* Agent Token */}
                  <div className="space-y-1">
                    <div className="flex items-center gap-1.5">
                      <KeyRound size={9} className="text-amber-400" />
                      <label className="text-[9px] font-black uppercase text-amber-400 tracking-widest">Agent Token</label>
                    </div>
                    <div className="flex gap-2">
                      <div className="flex-1 px-2.5 py-1.5 rounded-lg bg-amber-500/5 border border-amber-500/20 font-mono text-[10px] text-amber-300 flex items-center overflow-hidden">
                        <span className="truncate">{ak.key.slice(0, 18)}••••</span>
                      </div>
                      <Button size="icon" variant="ghost" className="h-7 w-7 rounded-lg shrink-0 text-amber-400" onClick={() => copyToClipboard(ak.key, 'Agent Token')}>
                        {copiedKey === ak.key ? <Check size={12} className="text-emerald-400" /> : <Copy size={12} />}
                      </Button>
                    </div>
                  </div>

                  {/* Balancer Token */}
                  <div className="space-y-1">
                    <div className="flex items-center gap-1.5">
                      <ShieldCheck size={9} className="text-sky-400" />
                      <label className="text-[9px] font-black uppercase text-sky-400 tracking-widest">Balancer Token</label>
                    </div>
                    <div className="flex gap-2">
                      <div className="flex-1 px-2.5 py-1.5 rounded-lg bg-sky-500/5 border border-sky-500/20 font-mono text-[10px] text-sky-300 flex items-center overflow-hidden">
                        <span className="truncate">{ak.balancer_token ? `${ak.balancer_token.slice(0, 18)}••••` : <span className="text-muted-foreground/40 italic">not available</span>}</span>
                      </div>
                      {ak.balancer_token && (
                        <Button size="icon" variant="ghost" className="h-7 w-7 rounded-lg shrink-0 text-sky-400" onClick={() => copyToClipboard(ak.balancer_token!, 'Balancer Token')}>
                          {copiedKey === ak.balancer_token ? <Check size={12} className="text-emerald-400" /> : <Copy size={12} />}
                        </Button>
                      )}
                    </div>
                  </div>

                  {/* Stats */}
                  <div className="flex justify-between items-center bg-muted/20 p-2 rounded-lg border border-border/50 mt-1">
                    <div className="flex flex-col">
                      <span className="text-[8px] font-black uppercase text-muted-foreground">Earnings</span>
                      <span className="text-xs font-black text-amber-400">{ak.credits_earned.toFixed(2)} φ</span>
                    </div>
                    <div className="flex flex-col items-end">
                      <span className="text-[8px] font-black uppercase text-muted-foreground">Reputation</span>
                      <span className="text-xs font-black text-blue-400">{ak.reputation.toFixed(1)}x</span>
                    </div>
                  </div>

                  {/* Visibility setting */}
                  <div className="flex items-center gap-3 pt-2 border-t border-border/30">
                    <span className="text-[9px] font-black uppercase text-muted-foreground tracking-widest shrink-0">Model Access</span>
                    <Select
                      value={ak.model_visibility || 'public'}
                      onValueChange={(val) => handleUpdateAgentVisibility(ak.key, val)}
                    >
                      <SelectTrigger className="h-6 text-[9px] font-black flex-1 border-border/40 bg-muted/30">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="public" className="text-[10px]">
                          <div className="flex items-center gap-1.5"><Globe size={10} className="text-emerald-400" /> Public — all users</div>
                        </SelectItem>
                        <SelectItem value="private" className="text-[10px]">
                          <div className="flex items-center gap-1.5"><Lock size={10} className="text-red-400" /> Private — my account only</div>
                        </SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
        </div>
      </div>

      {/* Token rotation flow */}
      {rotatingKey && (
        <RotateAgentKeyDialog
          agentKey={rotatingKey}
          onClose={() => setRotatingKey(null)}
          onSuccess={(updated) => {
            setRotatingKey(null);
            setRevealTitle('New Tokens Issued — Save These Now');
            setRevealKey(updated);
            load();
          }}
        />
      )}
      {revealKey && (
        <TokenRevealDialog
          agentKey={revealKey}
          title={revealTitle}
          onClose={() => { setRevealKey(null); setRevealTitle(undefined); }}
        />
      )}
    </>
  );
};

export default ProfilePage;
