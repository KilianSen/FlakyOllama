import { useState, useEffect } from 'react';
import { 
  Key, User as UserIcon, Shield, Copy, Check, Info, 
  Zap, Database, Activity, RefreshCw, Server, Plus, X
} from 'lucide-react';
import { toast } from 'sonner';

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Progress } from '@/components/ui/progress';
import { Separator } from '@/components/ui/separator';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog';
import sdk, { type ProfileResponse } from '@/api';

const ProfilePage = () => {
  const [profile, setProfile] = useState<ProfileResponse | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [copiedKey, setCopiedKey] = useState<string | null>(null);
  const [isApplying, setIsApplying] = useState(false);
  
  // New Key Dialog
  const [isNewKeyOpen, setIsNewKeyOpen] = useState(false);
  const [newKeyLabel, setNewKeyLabel] = useState('');
  const [newKeyQuota, setNewKeyQuota] = useState('1000000');

  const load = () => {
    setIsLoading(true);
    sdk.getMe()
      .then(setProfile)
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
      await sdk.createClientKey({ 
        label: newKeyLabel, 
        quota_limit: parseInt(newKeyQuota) 
      });
      toast.success('API Key created');
      setIsNewKeyOpen(false);
      setNewKeyLabel('');
      load();
    } catch (err: any) {
      toast.error(err.message);
    }
  };

  const requestAgentKey = async () => {
    setIsApplying(true);
    try {
      await sdk.createAgentKey({ label: `Agent for ${profile?.user.name}` });
      toast.success('Agent identity generated successfully');
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

  const { user, client_keys: keys, agent_keys: agents } = profile;
  const globalQuotaPercent = user.quota_limit > 0 ? (user.quota_used / user.quota_limit) * 100 : 0;
  const isOverGlobalQuota = user.quota_limit > 0 && user.quota_used >= user.quota_limit;

  return (
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
                <span className="text-[10px] font-black uppercase text-muted-foreground tracking-widest">Global Compute Credits</span>
                <div className="flex items-center gap-1.5 text-emerald-400">
                   <Zap size={14} />
                   <span className="text-sm font-black">{(user.quota_used / 100).toFixed(2)} φ earned</span>
                </div>
              </div>
            </CardContent>
          </Card>

          <Card className="border-border/40 bg-primary/5">
            <CardContent className="p-4 space-y-3">
              <div className="flex justify-between items-end">
                <label className="text-[10px] font-black uppercase text-primary tracking-widest">Account Quota</label>
                <span className={`text-[10px] font-black ${isOverGlobalQuota ? 'text-destructive' : 'text-primary'}`}>
                  {Math.round(globalQuotaPercent)}%
                </span>
              </div>
              <Progress value={user.quota_limit === -1 ? 0 : globalQuotaPercent} className="h-1.5" />
              <div className="flex justify-between text-[9px] font-bold text-muted-foreground uppercase">
                 <span>{user.quota_used.toLocaleString()} used</span>
                 <span>{user.quota_limit === -1 ? 'Unlimited' : user.quota_limit.toLocaleString()} total</span>
              </div>
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
                      <p className="text-[9px] text-muted-foreground italic">Limits this specific key's usage. contributes to global quota.</p>
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
            {keys.map((k) => {
              const keyPercent = k.quota_limit > 0 ? (k.quota_used / k.quota_limit) * 100 : 0;
              return (
                <Card key={k.key} className="bg-card/30 border-border/40 hover:border-primary/30 transition-colors group">
                  <CardContent className="p-4 flex flex-col sm:flex-row sm:items-center gap-4">
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
            Apply for Identity
          </Button>
        </div>
        
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          {agents.map((ak) => (
            <Card key={ak.key} className="border-border/40 bg-card/30 backdrop-blur-sm group overflow-hidden">
              <CardHeader className="p-4 pb-2">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <Server size={14} className="text-amber-400" />
                    <span className="text-xs font-black uppercase tracking-tight">{ak.label}</span>
                  </div>
                  <Badge variant="outline" className="text-[8px] font-black border-emerald-500/30 text-emerald-400 uppercase">
                    {ak.active ? 'Verified' : 'Inactive'}
                  </Badge>
                </div>
              </CardHeader>
              <CardContent className="p-4 pt-0 space-y-4">
                <div className="space-y-1.5 mt-2">
                  <label className="text-[9px] font-black uppercase text-muted-foreground tracking-widest">Identity Token</label>
                  <div className="flex gap-2">
                    <div className="flex-1 px-3 py-2 rounded-lg bg-black/40 border border-border/50 font-mono text-[11px] font-bold flex items-center justify-between group overflow-hidden">
                      <span className="truncate">{ak.key}</span>
                    </div>
                    <Button 
                      size="icon" variant="ghost"
                      className="h-8 w-8 rounded-lg shrink-0"
                      onClick={() => copyToClipboard(ak.key, 'Agent Token')}
                    >
                      {copiedKey === ak.key ? <Check size={14} /> : <Copy size={14} />}
                    </Button>
                  </div>
                </div>
                <div className="flex justify-between items-center bg-muted/20 p-2 rounded-lg border border-border/50">
                  <div className="flex flex-col">
                    <span className="text-[8px] font-black uppercase text-muted-foreground">Earnings</span>
                    <span className="text-xs font-black text-amber-400">{ak.credits_earned.toFixed(2)} φ</span>
                  </div>
                  <div className="flex flex-col items-end">
                    <span className="text-[8px] font-black uppercase text-muted-foreground">Reputation</span>
                    <span className="text-xs font-black text-blue-400">{ak.reputation.toFixed(1)}x</span>
                  </div>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      </div>
    </div>
  );
};

export default ProfilePage;
