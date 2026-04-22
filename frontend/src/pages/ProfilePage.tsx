import { useState, useEffect } from 'react';
import { 
  Key, User as UserIcon, Shield, Copy, Check, Info, 
  Zap, Database, Activity, RefreshCw 
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
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    sdk.getMe()
      .then(setProfile)
      .catch(err => toast.error('Failed to load profile: ' + err.message))
      .finally(() => setIsLoading(false));
  }, []);

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
    setCopied(true);
    toast.success('API Key copied to clipboard');
    setTimeout(() => setCopied(false), 2000);
  };

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-full">
        <RefreshCw className="animate-spin text-primary" size={24} />
      </div>
    );
  }

  if (!profile) return null;

  const { user, key } = profile;
  const quotaPercent = key.quota_limit > 0 ? (key.quota_used / key.quota_limit) * 100 : 0;
  const isOverQuota = key.quota_limit > 0 && key.quota_used >= key.quota_limit;

  return (
    <div className="p-8 max-w-5xl mx-auto space-y-8">
      <div className="flex flex-col gap-2">
        <h2 className="text-3xl font-black uppercase tracking-tighter">My Profile</h2>
        <p className="text-muted-foreground font-bold tracking-tight">Manage your account and access keys.</p>
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
            <CardDescription className="font-bold">Use this key to authenticate with the FlakyOllama API.</CardDescription>
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
                  onClick={() => copyToClipboard(key.key)}
                >
                  {copied ? <Check size={18} /> : <Copy size={18} />}
                </Button>
              </div>
              <p className="text-[10px] text-muted-foreground font-bold flex items-center gap-1.5">
                <Info size={12} className="text-amber-500" /> Keep this key secret. Never share it or commit it to code.
              </p>
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

      {/* Quick Access */}
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
