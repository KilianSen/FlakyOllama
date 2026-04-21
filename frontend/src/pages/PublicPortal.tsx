import React, { useState, useEffect } from 'react';
import { 
  Key, Coins, Zap, User, Box, 
  Info, TrendingUp
} from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Separator } from '@/components/ui/separator';
import { toast } from 'sonner';
import sdk, { type Catalog, type Identity } from '../api';

export const PublicPortal: React.FC = () => {
  const [token, setToken] = useState(localStorage.getItem('MY_PORTAL_TOKEN') || '');
  const [catalog, setCatalog] = useState<Catalog | null>(null);
  const [identity, setIdentity] = useState<Identity | null>(null);
  const [loading, setLoading] = useState(false);
  const [catLoading, setCatLoading] = useState(true);

  const loadCatalog = async () => {
    try {
      const data = await sdk.getCatalog();
      setCatalog(data);
    } catch (err) {
      console.error('Catalog failed');
    } finally {
      setCatLoading(false);
    }
  };

  const fetchIdentity = async () => {
    if (!token.trim()) return;
    setLoading(true);
    try {
      const data = await sdk.getMe(token);
      setIdentity(data);
      localStorage.setItem('MY_PORTAL_TOKEN', token);
      toast.success('Identity verified');
    } catch (err: any) {
      setIdentity(null);
      toast.error(err.message || 'Invalid token');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadCatalog();
    if (token) fetchIdentity();
  }, []);

  return (
    <div className="min-h-screen bg-background text-foreground p-6 md:p-12 max-w-5xl mx-auto space-y-12">
      {/* Header */}
      <div className="space-y-2">
        <div className="flex items-center gap-3">
          <div className="w-10 h-10 rounded-xl bg-primary/20 flex items-center justify-center">
            <Coins className="text-primary" size={24} />
          </div>
          <div>
            <h1 className="text-xl font-black uppercase tracking-tight">Public Terminal</h1>
            <p className="text-xs text-muted-foreground font-bold uppercase tracking-widest">Self-Service Portal & Catalog</p>
          </div>
        </div>
      </div>

      {/* Identity Login */}
      <section className="space-y-6">
        <div className="flex items-center gap-2 text-xs font-black uppercase tracking-widest text-muted-foreground">
          <Key size={14} /> Identity Verification
        </div>
        
        <Card className="bg-card border-border/50">
          <CardContent className="p-6">
            <div className="flex flex-col md:flex-row gap-4">
              <div className="flex-1 space-y-1.5">
                <Input 
                  type="password"
                  placeholder="Enter your API Key or Agent Identity Token..."
                  className="h-11 bg-muted/30 border-border/50 font-mono text-xs"
                  value={token}
                  onChange={e => setToken(e.target.value)}
                  onKeyDown={e => e.key === 'Enter' && fetchIdentity()}
                />
                <p className="text-[10px] text-muted-foreground ml-1 italic">Your token is stored locally in this browser for convenience.</p>
              </div>
              <Button 
                onClick={fetchIdentity} 
                disabled={loading}
                className="h-11 px-8 font-black uppercase text-xs tracking-widest shadow-lg shadow-primary/20"
              >
                {loading ? 'Verifying...' : 'Access Portal'}
              </Button>
            </div>

            {identity && (
              <div className="mt-8 grid grid-cols-1 md:grid-cols-3 gap-4 animate-in fade-in slide-in-from-top-4 duration-500">
                <div className="p-4 rounded-xl bg-muted/30 border border-border/50 space-y-2">
                  <div className="flex items-center gap-2 text-[10px] font-black uppercase text-muted-foreground">
                    {identity.type === 'client' ? <User size={12} className="text-blue-400" /> : <Zap size={12} className="text-amber-400" />}
                    {identity.type} Identity
                  </div>
                  <p className="text-sm font-black truncate">{identity.label}</p>
                </div>

                <div className="p-4 rounded-xl bg-muted/30 border border-border/50 space-y-2">
                  <div className="flex items-center gap-2 text-[10px] font-black uppercase text-muted-foreground">
                    <TrendingUp size={12} className="text-emerald-400" />
                    {identity.type === 'client' ? 'Quota Consumption' : 'Compute Contribution'}
                  </div>
                  <p className="text-sm font-black">
                    {identity.type === 'client' 
                      ? `${identity.data.quota_used.toLocaleString()} Tokens`
                      : `${(identity.data.credits_earned || 0).toLocaleString()} Credits`}
                  </p>
                </div>

                <div className="p-4 rounded-xl bg-emerald-500/10 border border-emerald-500/20 space-y-2">
                  <div className="flex items-center gap-2 text-[10px] font-black uppercase text-emerald-400">
                    <Coins size={12} />
                    Current Balance
                  </div>
                  <p className="text-sm font-black text-emerald-400">
                    {identity.type === 'client' 
                      ? `${(identity.data.credits || 0).toLocaleString()} φ`
                      : `${(identity.data.credits_earned || 0).toLocaleString()} φ`}
                  </p>
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      </section>

      {/* Model Catalog */}
      <section className="space-y-6">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2 text-xs font-black uppercase tracking-widest text-muted-foreground">
            <Box size={14} /> Cluster Model Catalog
          </div>
          {catalog && (
            <div className="flex gap-4">
              <Badge variant="outline" className="text-[9px] font-black uppercase border-amber-500/30 text-amber-500">
                Reward Multiplier: {catalog.global_reward_multiplier}x
              </Badge>
              <Badge variant="outline" className="text-[9px] font-black uppercase border-blue-500/30 text-blue-400">
                Cost Multiplier: {catalog.global_cost_multiplier}x
              </Badge>
            </div>
          )}
        </div>

        <Card className="bg-card border-border/50 overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow className="hover:bg-transparent border-border/50 bg-muted/20">
                <TableHead className="text-[10px] font-black uppercase py-4 pl-6">Model Identifier</TableHead>
                <TableHead className="text-[10px] font-black uppercase text-center">Incentive (Reward)</TableHead>
                <TableHead className="text-[10px] font-black uppercase text-center">Cost (Quota)</TableHead>
                <TableHead className="text-[10px] font-black uppercase text-right pr-6">Status</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {catLoading ? (
                <TableRow><TableCell colSpan={4} className="text-center py-12 text-muted-foreground text-xs italic">Syncing catalog...</TableCell></TableRow>
              ) : !catalog || catalog.models.length === 0 ? (
                <TableRow><TableCell colSpan={4} className="text-center py-12 text-muted-foreground text-xs italic">No models publicly listed</TableCell></TableRow>
              ) : catalog.models.map(model => (
                <TableRow key={model.name} className="border-border/40 hover:bg-muted/5 transition-colors">
                  <TableCell className="font-mono text-xs font-black pl-6">{model.name}</TableCell>
                  <TableCell className="text-center">
                    <Badge className="bg-amber-500/10 text-amber-500 border-amber-500/20 font-mono text-[10px] px-2 h-5">
                      {(model.reward_factor * catalog.global_reward_multiplier).toFixed(2)}x
                    </Badge>
                  </TableCell>
                  <TableCell className="text-center">
                    <Badge className="bg-blue-500/10 text-blue-500 border-blue-500/20 font-mono text-[10px] px-2 h-5">
                      {(model.cost_factor * catalog.global_cost_multiplier).toFixed(2)}x
                    </Badge>
                  </TableCell>
                  <TableCell className="text-right pr-6">
                    <div className="flex justify-end items-center gap-1.5 text-emerald-400 text-[10px] font-black uppercase tracking-tighter">
                       <CheckCircle2 size={10} /> Available
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Card>
      </section>

      {/* Footer Info */}
      <div className="p-6 rounded-2xl bg-muted/20 border border-border/30 flex gap-4">
        <Info className="text-muted-foreground shrink-0" size={20} />
        <div className="space-y-1">
          <p className="text-[10px] font-black uppercase tracking-widest text-muted-foreground">About Credits (φ)</p>
          <p className="text-[11px] text-muted-foreground/80 leading-relaxed">
            Credits are earned by providing compute power to the cluster via an Agent. 
            Consumers use credits to access high-performance models.
            The exchange rate is determined by the specific model's complexity factors and the current cluster multipliers.
          </p>
        </div>
      </div>
    </div>
  );
};
