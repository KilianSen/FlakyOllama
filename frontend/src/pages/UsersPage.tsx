import React, { useState, useEffect } from 'react';
import { User, Shield, Coins, Search, RefreshCw, MoreHorizontal, UserCog, Trash2, Zap } from 'lucide-react';
import { Card } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Label } from '@/components/ui/label';
import { toast } from 'sonner';
import sdk, { type UserWithKey, type QuotaTier, DEFAULT_TIERS } from '../api';

const TIER_LABELS: Record<QuotaTier, string> = {
  free: 'Free',
  standard: 'Standard',
  pro: 'Pro',
  unlimited: 'Unlimited',
  custom: 'Custom',
};

const TIER_COLORS: Record<QuotaTier, string> = {
  free: 'text-muted-foreground border-border',
  standard: 'text-blue-400 border-blue-500/30 bg-blue-500/5',
  pro: 'text-primary border-primary/30 bg-primary/5',
  unlimited: 'text-emerald-400 border-emerald-500/30 bg-emerald-500/5',
  custom: 'text-amber-400 border-amber-500/30 bg-amber-500/5',
};

const fmtLimit = (n: number) => n === -1 ? '∞' : n >= 1_000_000 ? `${(n/1_000_000).toFixed(1)}M` : n >= 1_000 ? `${(n/1_000).toFixed(0)}k` : String(n);

interface QuotaState {
  tier: QuotaTier;
  total: number;
  daily: number;
  weekly: number;
  monthly: number;
}

export const UsersPage: React.FC = () => {
  const [users, setUsers] = useState<UserWithKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');
  const [selectedUser, setSelectedUser] = useState<UserWithKey | null>(null);
  const [quota, setQuota] = useState<QuotaState>({ tier: 'custom', total: -1, daily: -1, weekly: -1, monthly: -1 });
  const [isQuotaDialogOpen, setIsQuotaDialogOpen] = useState(false);

  const refresh = async () => {
    setLoading(true);
    try {
      setUsers(await sdk.getUsers());
    } catch (err: any) {
      toast.error('Failed to fetch users: ' + err.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { refresh(); }, []);

  const openQuotaDialog = (u: UserWithKey) => {
    setSelectedUser(u);
    setQuota({
      tier: (u.user.quota_tier || 'custom') as QuotaTier,
      total: u.user.quota_limit ?? -1,
      daily: u.user.daily_quota_limit ?? -1,
      weekly: u.user.weekly_quota_limit ?? -1,
      monthly: u.user.monthly_quota_limit ?? -1,
    });
    setIsQuotaDialogOpen(true);
  };

  const applyTier = (tier: QuotaTier) => {
    const limits = DEFAULT_TIERS[tier];
    setQuota({ tier, total: limits.total, daily: limits.daily, weekly: limits.weekly, monthly: limits.monthly });
  };

  const handleUpdateQuota = async () => {
    if (!selectedUser) return;
    try {
      await sdk.updateUserQuota(selectedUser.user.id, {
        quota_tier: quota.tier,
        quota_limit: quota.total,
        daily_quota_limit: quota.daily,
        weekly_quota_limit: quota.weekly,
        monthly_quota_limit: quota.monthly,
      });
      toast.success(`Quota updated for ${selectedUser.user.email}`);
      setIsQuotaDialogOpen(false);
      refresh();
    } catch (err: any) {
      toast.error(err.message);
    }
  };

  const handleDeleteUser = async (u: UserWithKey) => {
    if (!confirm(`Permanently delete ${u.user.email}? All keys and history will be lost.`)) return;
    try {
      await sdk.deleteUser(u.user.id);
      toast.success('User deleted');
      refresh();
    } catch (err: any) {
      toast.error('Failed to delete user: ' + err.message);
    }
  };

  const filtered = users.filter(u =>
    u.user.email.toLowerCase().includes(search.toLowerCase()) ||
    u.user.name?.toLowerCase().includes(search.toLowerCase()) ||
    u.user.id.includes(search)
  );

  return (
    <div className="p-8 space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-black uppercase tracking-tight">User Management</h2>
          <p className="text-muted-foreground text-xs font-bold uppercase tracking-widest mt-1">Manage cluster permissions and quotas</p>
        </div>
        <Button onClick={refresh} disabled={loading} variant="outline" size="sm" className="font-black uppercase text-[10px] tracking-widest gap-2">
          <RefreshCw size={12} className={loading ? 'animate-spin' : ''} /> Refresh
        </Button>
      </div>

      <div className="relative max-w-sm">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" size={14} />
        <Input placeholder="Search users..." value={search} onChange={e => setSearch(e.target.value)} className="pl-10 font-bold text-xs" />
      </div>

      <Card className="border-border/50 bg-card/50 overflow-hidden">
        <Table>
          <TableHeader>
            <TableRow className="border-border/50 hover:bg-transparent">
              <TableHead className="text-[10px] font-black uppercase tracking-widest py-4 pl-6">User</TableHead>
              <TableHead className="text-[10px] font-black uppercase tracking-widest py-4">Role</TableHead>
              <TableHead className="text-[10px] font-black uppercase tracking-widest py-4">Tier</TableHead>
              <TableHead className="text-[10px] font-black uppercase tracking-widest py-4">Tokens Used</TableHead>
              <TableHead className="text-[10px] font-black uppercase tracking-widest py-4">Spent / Earned</TableHead>
              <TableHead className="text-right text-[10px] font-black uppercase tracking-widest py-4 pr-6">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {filtered.length === 0 ? (
              <TableRow>
                <TableCell colSpan={6} className="h-32 text-center text-muted-foreground font-bold text-xs uppercase tracking-widest">
                  {loading ? 'Loading users...' : 'No users found'}
                </TableCell>
              </TableRow>
            ) : filtered.map(u => {
              const tier = (u.user.quota_tier || 'custom') as QuotaTier;
              return (
                <TableRow key={u.user.id} className="border-border/40 hover:bg-muted/5 group">
                  <TableCell className="pl-6 py-4">
                    <div className="flex items-center gap-3">
                      <div className="w-8 h-8 rounded-full bg-primary/10 flex items-center justify-center text-primary text-[10px] font-black border border-primary/20">
                        {u.user.name?.substring(0, 2).toUpperCase() || 'U'}
                      </div>
                      <div>
                        <p className="text-[11px] font-black uppercase leading-none">{u.user.name || 'Anonymous'}</p>
                        <p className="text-[10px] text-muted-foreground font-bold mt-1">{u.user.email}</p>
                      </div>
                    </div>
                  </TableCell>
                  <TableCell>
                    {u.user.is_admin ? (
                      <Badge className="bg-amber-500/10 text-amber-500 border-amber-500/20 text-[9px] font-black uppercase px-1.5 h-5 gap-1">
                        <Shield size={10} /> Admin
                      </Badge>
                    ) : (
                      <Badge variant="secondary" className="text-[9px] font-black uppercase px-1.5 h-5 gap-1">
                        <User size={10} /> User
                      </Badge>
                    )}
                  </TableCell>
                  <TableCell>
                    <Badge variant="outline" className={`text-[9px] font-black uppercase px-1.5 h-5 ${TIER_COLORS[tier]}`}>
                      {TIER_LABELS[tier]}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-col gap-0.5">
                      <span className="text-[11px] font-black">{(u.user.quota_used / 1e6).toFixed(2)}M</span>
                      <span className="text-[9px] text-muted-foreground font-bold uppercase">
                        / {u.user.quota_limit === -1 ? '∞' : `${(u.user.quota_limit / 1e6).toFixed(1)}M`} total
                      </span>
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-col gap-0.5">
                      <div className="flex items-center gap-1 text-red-400">
                        <Coins size={10} />
                        <span className="text-[10px] font-black">{Math.abs(u.key?.credits || 0).toFixed(2)} φ spent</span>
                      </div>
                      {(u.agent_earnings || 0) > 0 && (
                        <div className="flex items-center gap-1 text-amber-400">
                          <Zap size={10} />
                          <span className="text-[10px] font-black">{u.agent_earnings.toFixed(2)} φ earned</span>
                        </div>
                      )}
                    </div>
                  </TableCell>
                  <TableCell className="text-right pr-6">
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button variant="ghost" size="icon" className="h-8 w-8 text-muted-foreground">
                          <MoreHorizontal size={14} />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end" className="w-48">
                        <DropdownMenuLabel className="text-[10px] font-black uppercase tracking-widest opacity-50">Manage</DropdownMenuLabel>
                        <DropdownMenuItem className="font-bold cursor-pointer" onClick={() => openQuotaDialog(u)}>
                          <UserCog className="mr-2 h-4 w-4" /> Adjust Quota
                        </DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem className="font-bold text-destructive cursor-pointer" onClick={() => handleDeleteUser(u)}>
                          <Trash2 className="mr-2 h-4 w-4" /> Delete Account
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      </Card>

      <Dialog open={isQuotaDialogOpen} onOpenChange={setIsQuotaDialogOpen}>
        <DialogContent className="sm:max-w-[520px]">
          <DialogHeader>
            <DialogTitle className="text-lg font-black uppercase tracking-tight text-primary flex items-center gap-2">
              <UserCog size={18} /> Adjust Quota — {selectedUser?.user.email}
            </DialogTitle>
            <DialogDescription className="text-xs font-bold uppercase tracking-widest text-muted-foreground">
              Apply a tier preset or configure individual limits. Agent earnings offset all quotas.
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-5 py-2">
            {/* Tier Presets */}
            <div className="space-y-2">
              <Label className="text-[10px] font-black uppercase tracking-widest text-muted-foreground">Tier Preset</Label>
              <div className="grid grid-cols-5 gap-1.5">
                {(Object.keys(TIER_LABELS) as QuotaTier[]).map(t => (
                  <button
                    key={t}
                    onClick={() => applyTier(t)}
                    className={`px-2 py-2 rounded-lg border text-[9px] font-black uppercase tracking-widest transition-all ${
                      quota.tier === t ? TIER_COLORS[t] + ' ring-1 ring-current' : 'border-border text-muted-foreground hover:border-muted-foreground'
                    }`}
                  >
                    {TIER_LABELS[t]}
                  </button>
                ))}
              </div>
              {quota.tier !== 'custom' && (
                <p className="text-[9px] text-muted-foreground italic">
                  {TIER_LABELS[quota.tier]}: {fmtLimit(quota.daily)}/day · {fmtLimit(quota.weekly)}/week · {fmtLimit(quota.monthly)}/month · {fmtLimit(quota.total)} total
                </p>
              )}
            </div>

            {/* Individual limits */}
            <div className="grid grid-cols-2 gap-3">
              {([
                { key: 'daily', label: 'Daily Limit' },
                { key: 'weekly', label: 'Weekly Limit' },
                { key: 'monthly', label: 'Monthly Limit' },
                { key: 'total', label: 'Total (Lifetime)' },
              ] as { key: keyof Omit<QuotaState, 'tier'>; label: string }[]).map(({ key, label }) => (
                <div key={key} className="space-y-1">
                  <Label className="text-[9px] font-black uppercase tracking-widest text-muted-foreground">{label}</Label>
                  <Input
                    type="number"
                    value={quota[key]}
                    onChange={e => setQuota(q => ({ ...q, tier: 'custom', [key]: parseInt(e.target.value) || -1 }))}
                    className="font-mono text-xs h-8"
                  />
                </div>
              ))}
            </div>
            <p className="text-[9px] text-muted-foreground italic">Use -1 for unlimited. Agent earned credits reduce effective usage across all windows.</p>
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => setIsQuotaDialogOpen(false)} className="font-black uppercase text-xs">Cancel</Button>
            <Button onClick={handleUpdateQuota} className="font-black uppercase text-xs gap-2">
              <UserCog size={12} /> Apply Quota
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
};
