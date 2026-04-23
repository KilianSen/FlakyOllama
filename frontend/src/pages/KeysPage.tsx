import React, { useState, useEffect } from 'react';
import { Edit2, Check, X } from 'lucide-react';
import { Card } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { toast } from 'sonner';
import sdk, { type UserWithKey } from '../api';

export const KeysPage: React.FC = () => {
  const [users, setUsers] = useState<UserWithKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [editingQuota, setEditingQuota] = useState<string | null>(null);
  const [quotaValue, setQuotaValue] = useState<string>('');

  const load = async () => {
    setLoading(true);
    try {
      const data = await sdk.getUsers();
      setUsers(data || []);
    } catch (err) {
      toast.error('Failed to load users');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { load(); }, []);

  const saveQuota = async (userId: string) => {
    try {
      await sdk.updateUserQuota(userId, parseInt(quotaValue));
      toast.success('Quota updated');
      setEditingQuota(null);
      load();
    } catch (err: any) {
      toast.error(err.message);
    }
  };

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-sm font-black uppercase tracking-widest text-foreground">User Management</h2>
          <p className="text-[11px] text-muted-foreground mt-0.5">Oversee cluster users, manage quotas, and verify identities.</p>
        </div>
        <Button variant="outline" size="sm" className="h-8 text-[10px] font-black uppercase" onClick={load} disabled={loading}>
           {loading ? 'Syncing...' : 'Refresh Users'}
        </Button>
      </div>

      <Card className="bg-card border-border/50 overflow-hidden">
        <Table>
          <TableHeader>
            <TableRow className="hover:bg-transparent border-border/50 bg-muted/20">
              <TableHead className="text-[10px] font-black uppercase py-4 pl-6">User / Identity</TableHead>
              <TableHead className="text-[10px] font-black uppercase text-center">Account Type</TableHead>
              <TableHead className="text-[10px] font-black uppercase text-center">Token Quota</TableHead>
              <TableHead className="text-[10px] font-black uppercase text-center">Usage</TableHead>
              <TableHead className="text-[10px] font-black uppercase text-right pr-6">Status</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {users.length === 0 ? (
              <TableRow><TableCell colSpan={5} className="text-center py-12 text-muted-foreground text-xs italic">No users registered in cluster</TableCell></TableRow>
            ) : users.map(u => (
              <TableRow key={u.user.id} className="border-border/40 hover:bg-muted/5 transition-colors">
                <TableCell className="py-4 pl-6">
                  <div className="flex items-center gap-3">
                    <div className="w-8 h-8 rounded-lg bg-primary/10 border border-primary/20 flex items-center justify-center text-primary font-black text-[10px]">
                      {u.user.name.substring(0, 2).toUpperCase()}
                    </div>
                    <div>
                      <p className="text-xs font-black tracking-tight">{u.user.name}</p>
                      <p className="text-[10px] text-muted-foreground font-mono">{u.user.email}</p>
                    </div>
                  </div>
                </TableCell>
                <TableCell className="text-center">
                  {u.user.is_admin ? (
                    <Badge className="bg-primary/20 text-primary border-primary/30 font-black text-[9px] px-2 h-5">ADMIN</Badge>
                  ) : (
                    <Badge variant="secondary" className="font-black text-[9px] px-2 h-5">USER</Badge>
                  )}
                </TableCell>
                <TableCell className="text-center">
                  {editingQuota === u.user.id ? (
                    <div className="flex items-center justify-center gap-1">
                      <Input 
                        className="h-7 w-24 text-[10px] font-bold py-0" 
                        value={quotaValue} 
                        onChange={e => setQuotaValue(e.target.value)}
                        autoFocus
                      />
                      <Button size="icon" variant="ghost" className="h-7 w-7 text-emerald-400" onClick={() => saveQuota(u.user.id)}>
                        <Check size={14} />
                      </Button>
                      <Button size="icon" variant="ghost" className="h-7 w-7 text-red-400" onClick={() => setEditingQuota(null)}>
                        <X size={14} />
                      </Button>
                    </div>
                  ) : (
                    <div className="flex items-center justify-center gap-2 group">
                      <span className="text-xs font-mono font-bold">
                        {u.key.quota_limit === -1 ? 'Unlimited' : u.key.quota_limit.toLocaleString()}
                      </span>
                      <button 
                        onClick={() => { setEditingQuota(u.user.id); setQuotaValue(u.key.quota_limit.toString()); }}
                        className="opacity-0 group-hover:opacity-100 transition-opacity text-muted-foreground hover:text-primary"
                      >
                        <Edit2 size={12} />
                      </button>
                    </div>
                  )}
                </TableCell>
                <TableCell className="text-center">
                  <div className="w-24 mx-auto space-y-1">
                    <div className="flex justify-between text-[9px] font-black uppercase text-muted-foreground">
                      <span>Used</span>
                      <span>{u.key.quota_limit > 0 ? Math.round((u.key.quota_used / u.key.quota_limit) * 100) : 0}%</span>
                    </div>
                    <div className="h-1 bg-muted rounded-full overflow-hidden">
                      <div 
                        className="h-full bg-primary" 
                        style={{ width: `${u.key.quota_limit > 0 ? Math.min(100, (u.key.quota_used / u.key.quota_limit) * 100) : 0}%` }} 
                      />
                    </div>
                  </div>
                </TableCell>
                <TableCell className="text-right pr-6">
                   <div className="flex justify-end">
                     <Badge className={u.key.active ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/20 font-black text-[9px]' : 'bg-red-500/10 text-red-400 font-black text-[9px]'}>
                       {u.key.active ? 'ACTIVE' : 'LOCKED'}
                     </Badge>
                   </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Card>
    </div>
  );
};
