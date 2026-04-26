import React, { useState, useEffect } from 'react';
import { User, Shield, Coins, Search, RefreshCw, MoreHorizontal, UserCog, Trash2 } from 'lucide-react';
import { Card } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Label } from '@/components/ui/label';
import { toast } from 'sonner';
import sdk, { type UserWithKey } from '../api';

export const UsersPage: React.FC = () => {
  const [users, setUsers] = useState<UserWithKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');
  const [selectedUser, setSelectedUser] = useState<UserWithKey | null>(null);
  const [newQuota, setNewQuota] = useState<number>(0);
  const [isQuotaDialogOpen, setIsQuotaDialogOpen] = useState(false);

  const refresh = async () => {
    setLoading(true);
    try {
      const data = await sdk.getUsers();
      setUsers(data);
    } catch (err: any) {
      toast.error('Failed to fetch users: ' + err.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { refresh(); }, []);

  const handleUpdateQuota = async () => {
    if (!selectedUser) return;
    try {
      await sdk.updateUserQuota(selectedUser.user.id, newQuota);
      toast.success(`Quota updated for ${selectedUser.user.email}`);
      setIsQuotaDialogOpen(false);
      refresh();
    } catch (err: any) {
      toast.error(err.message);
    }
  };

  const handleDeleteUser = async (u: UserWithKey) => {
    if (!confirm(`Are you sure you want to PERMANENTLY delete user ${u.user.email}? All associated keys and usage history will be lost.`)) return;
    try {
      await sdk.deleteUser(u.user.id);
      toast.success('User deleted successfully');
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
          <RefreshCw size={12} className={loading ? 'animate-spin' : ''} />
          Refresh
        </Button>
      </div>

      <div className="flex items-center gap-4">
        <div className="relative flex-1 max-w-sm">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" size={14} />
          <Input 
            placeholder="Search users..." 
            value={search}
            onChange={e => setSearch(e.target.value)}
            className="pl-10 font-bold text-xs"
          />
        </div>
      </div>

      <Card className="border-border/50 bg-card/50 overflow-hidden">
        <Table>
          <TableHeader>
            <TableRow className="border-border/50 hover:bg-transparent">
              <TableHead className="text-[10px] font-black uppercase tracking-widest py-4 pl-6">User</TableHead>
              <TableHead className="text-[10px] font-black uppercase tracking-widest py-4">Role</TableHead>
              <TableHead className="text-[10px] font-black uppercase tracking-widest py-4">Current Quota</TableHead>
              <TableHead className="text-[10px] font-black uppercase tracking-widest py-4">Credits</TableHead>
              <TableHead className="text-right text-[10px] font-black uppercase tracking-widest py-4 pr-6">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {filtered.length === 0 ? (
              <TableRow>
                <TableCell colSpan={5} className="h-32 text-center text-muted-foreground font-bold text-xs uppercase tracking-widest">
                  {loading ? 'Loading users...' : 'No users found'}
                </TableCell>
              </TableRow>
            ) : filtered.map(u => (
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
                  <div className="flex flex-col gap-0.5">
                    <span className="text-[11px] font-black">
                      {u.key.quota_limit === -1 ? '∞ UNLIMITED' : `${(u.key.quota_limit / 1e6).toFixed(1)}M tokens`}
                    </span>
                    <span className="text-[9px] text-muted-foreground font-bold uppercase">
                      {((u.key.quota_used || 0) / 1e6).toFixed(1)}M used
                    </span>
                  </div>
                </TableCell>
                <TableCell>
                  <div className="flex items-center gap-1.5 text-amber-400">
                    <Coins size={12} />
                    <span className="text-[11px] font-black">{(u.key.credits || 0).toLocaleString()} φ</span>
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
                      <DropdownMenuItem className="font-bold cursor-pointer" onClick={() => {
                        setSelectedUser(u);
                        setNewQuota(u.key.quota_limit);
                        setIsQuotaDialogOpen(true);
                      }}>
                        <UserCog className="mr-2 h-4 w-4" />
                        <span>Adjust Quota</span>
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem className="font-bold text-destructive cursor-pointer" onClick={() => handleDeleteUser(u)}>
                        <Trash2 className="mr-2 h-4 w-4" />
                        <span>Delete Account</span>
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Card>

      <Dialog open={isQuotaDialogOpen} onOpenChange={setIsQuotaDialogOpen}>
        <DialogContent className="sm:max-w-[425px]">
          <DialogHeader>
            <DialogTitle className="text-lg font-black uppercase tracking-tight text-primary flex items-center gap-2">
                <Coins className="text-amber-400" /> Adjust User Quota
            </DialogTitle>
            <DialogDescription className="text-xs font-bold uppercase tracking-widest text-muted-foreground">
              Modify the token limit for {selectedUser?.user.email}
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 py-4">
            <div className="space-y-2">
              <Label htmlFor="quota" className="text-[10px] font-black uppercase tracking-widest text-muted-foreground">Limit (tokens, -1 for unlimited)</Label>
              <Input
                id="quota"
                type="number"
                value={newQuota}
                onChange={e => setNewQuota(parseInt(e.target.value))}
                className="font-black"
              />
              <p className="text-[9px] text-muted-foreground font-medium uppercase tracking-tight">
                  Suggested: 1,000,000 (1M), 10,000,000 (10M), or -1
              </p>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setIsQuotaDialogOpen(false)} className="font-black uppercase text-xs">Cancel</Button>
            <Button onClick={handleUpdateQuota} className="font-black uppercase text-xs gap-2">
                Update Quota
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
};
