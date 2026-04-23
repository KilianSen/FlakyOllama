import { useState, useEffect } from 'react';
import { 
  Key, User as UserIcon, Shield, Copy, Check, Info, 
  Zap, Database, Activity, RefreshCw, Server, Plus
} from 'lucide-react';
import { toast } from 'sonner';

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Progress } from '@/components/ui/progress';
import { Separator } from '@/components/ui/separator';
import sdk, { type ProfileResponse } from '@/api';

const ProfilePage = () => {
  const [profile, setProfile] = useState<ProfileResponse | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [copiedKey, setCopiedKey] = useState<string | null>(null);
  const [isApplying, setIsApplying] = useState(false);

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

  const { user, client_key: key, agent_keys: agents } = profile;
  const quotaPercent = key.quota_limit > 0 ? (key.quota_used / key.quota_limit) * 100 : 0;
  const isOverQuota = key.quota_limit > 0 && key.quota_used >= key.quota_limit;

  return (
    <div className="p-8 max-w-5xl mx-auto space-y-8 pb-20">
      <div className="flex flex-col gap-2">
        <h2 className="text-3xl font-black uppercase tracking-tighter">My Profile</h2>
        <p className="text-muted-foreground font-bold tracking-tight">Manage your account, access keys, and compute contributions.</p>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
        {/* User Info */}
        <Card className="md:col-span-1 border-border/40 shadow-xl shadow-black/20 bg-card/50 backdrop-blur-md">
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
              <span className="text-[10px] font-black uppercase text-muted-foreground tracking-widest">Internal ID</span>
              <span className="text-xs font-mono font-bold text-muted-foreground truncate">{user.id}</span>
            </div>
          </CardContent>
        </Card>

        {/* API Access */}
        <Card className="md:col-span-2 border-border/40 shadow-xl shadow-black/20 bg-card/50 backdrop-blur-md overflow-hidden relative">
          <div className="absolute top-0 right-0 p-8 opacity-[0.03] pointer-events-none">
            <Key size={120} className="rotate-12" />
          </div>
          
          <CardHeader>
            <CardTitle className="text-xl font-black uppercase tracking-tight flex items-center gap-2">
              <Key size={18} className="text-primary" /> API Access
            </CardTitle>
            <CardDescription className="font-bold">Use this key to authenticate with the FlakyOllama API for inference.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-6">
            <div className="space-y-3">
              <label className="text-xs font-black uppercase text-muted-foreground tracking-widest">Personal API Key</label>
              <div className="flex gap-2">
                <div className="flex-1 px-4 py-3 rounded-xl bg-black/40 border border-border/50 font-mono text-sm font-bold flex items-center justify-between group overflow-hidden">
                  <span className="truncate">{key.key}</span>
                  <div className="w-1.5 h-1.5 rounded-full bg-primary animate-pulse shrink-0" />
                </div>
                <Button 
                  size="icon" 
                  className="h-[46px] w-[46px] rounded-xl shrink-0 transition-all hover:scale-105"
                  onClick={() => copyToClipboard(key.key, 'API Key')}
                >
                  {copiedKey === key.key ? <Check size={18} /> : <Copy size={18} />}
                </Button>
              </div>
            </div>

            <Separator className="bg-border/50" />

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <div className="space-y-3">
                <div className="flex justify-between items-end">
                  <label className="text-xs font-black uppercase text-muted-foreground tracking-widest">Quota Usage</label>
                  <span className={`text-[10px] font-black ${isOverQuota ? 'text-destructive' : 'text-primary'}`}>
                    {key.quota_used.toLocaleString()} / {key.quota_limit === -1 ? '∞' : key.quota_limit.toLocaleString()}
                  </span>
                </div>
                <Progress value={key.quota_limit === -1 ? 0 : quotaPercent} className={`h-2 ${isOverQuota ? 'bg-destructive/10' : ''}`} />
                <p className="text-[10px] text-muted-foreground font-bold italic">Tokens consumed across all models.</p>
              </div>

              <div className="space-y-3">
                <div className="flex justify-between items-end">
                  <label className="text-xs font-black uppercase text-muted-foreground tracking-widest">Compute Credits</label>
                  <span className="text-[10px] font-black text-emerald-400">
                    {key.credits.toFixed(2)} CREDITS
                  </span>
                </div>
                <div className="h-2 rounded-full bg-emerald-500/20 overflow-hidden">
                   <div className="h-full bg-emerald-500 w-[70%]" /> {/* Mock fill */}
                </div>
                <p className="text-[10px] text-muted-foreground font-bold italic">Available for high-priority routing.</p>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Agent Identities */}
      <div className="space-y-6">
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
        <p className="text-[11px] text-muted-foreground font-bold italic bg-amber-500/5 p-4 rounded-xl border border-amber-500/10">
          <Info size={12} className="inline mr-2 text-amber-400" />
          Use an Agent Identity token to connect your local hardware to the cluster. Credits earned by your agents are pooled into your account balance.
        </p>
      </div>

      {/* Quick Access Stats */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        {[
          { icon: Zap, label: 'Fastest Node', value: 'node-aphenia', color: 'text-primary' },
          { icon: Database, label: 'Default Model', value: 'llama3.2:1b', color: 'text-amber-400' },
          { icon: Activity, label: 'API Status', value: 'Operational', color: 'text-emerald-400' },
        ].map((item, i) => (
          <Card key={i} className="border-border/40 bg-card/30 backdrop-blur-sm">
            <CardContent className="p-4 flex items-center gap-4">
              <div className={`p-2 rounded-lg bg-background/50 border border-border/50 ${item.color}`}>
                <item.icon size={16} />
              </div>
              <div>
                <p className="text-[9px] font-black uppercase text-muted-foreground tracking-widest">{item.label}</p>
                <p className="text-xs font-bold">{item.value}</p>
              </div>
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  );
};

export default ProfilePage;
