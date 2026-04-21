import React, { useState, useEffect } from 'react';
import { Search, Download, Trash2, CheckCircle2, XCircle, Clock, Box, Database, ExternalLink, ShieldAlert, Cpu, Zap } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Badge } from '@/components/ui/badge';
import { Tabs, TabsContent, List, TabsTrigger } from '@/components/ui/tabs';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Separator } from '@/components/ui/separator';
import { toast } from 'sonner';
import sdk, { type ModelRequest } from '../api';
import { useCluster } from '../ClusterContext';
import { formatBytes } from '../lib/modelUtils';

// Common models for the browser
const POPULAR_MODELS = [
  { name: 'llama3:8b', size: '4.7GB', desc: 'Meta Meta Llama 3, the most capable open-source model at this scale.', family: 'llama' },
  { name: 'llama3:70b', size: '40GB', desc: 'High-performance version of Llama 3 for complex tasks.', family: 'llama' },
  { name: 'mistral:7b', size: '4.1GB', desc: 'Mistral 7B is a high-performance transformer model.', family: 'mistral' },
  { name: 'mixtral:8x7b', size: '26GB', desc: 'Mistral AI mixture-of-experts model.', family: 'mistral' },
  { name: 'phi3:latest', size: '2.3GB', desc: 'Microsoft Phi-3 Mini, a 3.8B parameter lightweight model.', family: 'phi' },
  { name: 'gemma:7b', size: '5.0GB', desc: 'Google Gemma is a family of lightweight, state-of-the-art open models.', family: 'gemma' },
  { name: 'codellama:latest', size: '4.8GB', desc: 'A model for generating and discussing code.', family: 'llama' },
  { name: 'deepseek-coder:latest', size: '4.5GB', desc: 'State-of-the-art code completion and generation.', family: 'deepseek' },
  { name: 'llava:latest', size: '4.5GB', desc: 'A multimodal model that can see and talk.', family: 'llava' },
];

export const RegistryPage: React.FC = () => {
  const { status, refresh } = useCluster();
  const [search, setSearch] = useState('');
  const [requests, setRequests] = useState<ModelRequest[]>([]);
  const [loading, setLoading] = useState(false);
  const [activeTab, setActiveTab] = useState('browser');

  const loadRequests = async () => {
    try {
      const data = await sdk.getModelRequests('pending');
      setRequests(data);
    } catch (err) {
      console.error('Failed to load requests:', err);
    }
  };

  useEffect(() => {
    loadRequests();
    const interval = setInterval(loadRequests, 10000);
    return () => clearInterval(interval);
  }, []);

  const handleApprove = async (id: string) => {
    try {
      await sdk.approveModelRequest(id);
      toast.success('Request approved and triggered');
      loadRequests();
      refresh();
    } catch (err: any) {
      toast.error(err.message || 'Failed to approve');
    }
  };

  const handleDecline = async (id: string) => {
    try {
      await sdk.declineModelRequest(id);
      toast.success('Request declined');
      loadRequests();
    } catch (err: any) {
      toast.error(err.message || 'Failed to decline');
    }
  };

  const handlePull = async (model: string, targetNode?: string) => {
    try {
      const res = await sdk.pullModel(model, targetNode === 'cluster' ? undefined : targetNode);
      if (res.status === 'approval_pending') {
        toast.info('Request submitted for manual approval');
        loadRequests();
      } else {
        toast.success(`Pull triggered for ${model}`);
      }
    } catch (err: any) {
      toast.error(err.message || 'Failed to trigger pull');
    }
  };

  const filteredModels = POPULAR_MODELS.filter(m => 
    m.name.toLowerCase().includes(search.toLowerCase()) ||
    m.desc.toLowerCase().includes(search.toLowerCase())
  );

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-sm font-black uppercase tracking-widest">Model Management</h2>
          <p className="text-[11px] text-muted-foreground mt-0.5">Browse the fleet registry and manage manual approvals</p>
        </div>
        {requests.length > 0 && (
          <Badge className="bg-amber-500/20 text-amber-400 border-amber-500/30 font-black animate-pulse">
            {requests.length} Pending Actions
          </Badge>
        )}
      </div>

      <Tabs defaultValue="browser" onValueChange={setActiveTab} className="space-y-4">
        <div className="flex items-center justify-between">
          <div className="flex bg-muted/30 p-1 rounded-lg border border-border/50">
             <button 
              onClick={() => setActiveTab('browser')}
              className={`px-4 py-1.5 text-[10px] font-black uppercase tracking-wider rounded-md transition-all ${activeTab === 'browser' ? 'bg-background text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground'}`}
             >
               Model Browser
             </button>
             <button 
              onClick={() => setActiveTab('requests')}
              className={`px-4 py-1.5 text-[10px] font-black uppercase tracking-wider rounded-md transition-all flex items-center gap-2 ${activeTab === 'requests' ? 'bg-background text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground'}`}
             >
               Pull Requests
               {requests.length > 0 && <span className="w-1.5 h-1.5 rounded-full bg-amber-500" />}
             </button>
          </div>

          {activeTab === 'browser' && (
            <div className="relative w-72">
              <Search className="absolute left-2.5 top-2.5 h-3.5 w-3.5 text-muted-foreground" />
              <Input
                placeholder="Search models..."
                className="pl-9 h-9 bg-card border-border/50 text-xs"
                value={search}
                onChange={e => setSearch(e.target.value)}
              />
            </div>
          )}
        </div>

        <TabsContent value="browser" className="m-0">
          <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
            {filteredModels.map(model => (
              <Card key={model.name} className="bg-card border-border/50 hover:border-primary/30 transition-colors group">
                <CardContent className="p-5">
                  <div className="flex items-start justify-between mb-3">
                    <div className="flex items-center gap-3">
                      <div className="p-2 rounded-lg bg-muted/50 group-hover:bg-primary/10 transition-colors">
                        <Box size={18} className="group-hover:text-primary transition-colors" />
                      </div>
                      <div>
                        <p className="text-sm font-black tracking-tight">{model.name}</p>
                        <p className="text-[10px] text-muted-foreground font-bold">{model.size}</p>
                      </div>
                    </div>
                    <div>
                       <Select onValueChange={(val) => handlePull(model.name, val)}>
                          <SelectTrigger className="h-8 w-32 text-[10px] font-black uppercase bg-muted/50 border-border/50">
                            <div className="flex items-center gap-2">
                              <Download size={12} />
                              <span className="truncate text-left">Deploy...</span>
                            </div>
                          </SelectTrigger>
                          <SelectContent>
                             <SelectItem value="cluster" className="text-[10px] font-bold">全 Cluster-wide</SelectItem>
                             <Separator className="my-1" />
                             {Object.values(status?.nodes || {}).map(n => (
                               <SelectItem key={n.id} value={n.id} className="text-[10px] font-bold">
                                 {n.has_gpu ? '⚡' : '●'} {n.id}
                               </SelectItem>
                             ))}
                          </SelectContent>
                       </Select>
                    </div>
                  </div>
                  <p className="text-[11px] text-muted-foreground leading-relaxed line-clamp-2 mb-4">
                    {model.desc}
                  </p>
                  <div className="flex items-center justify-between pt-4 border-t border-border/30">
                    <Badge variant="outline" className="text-[8px] font-black uppercase tracking-tighter opacity-60">
                      {model.family}
                    </Badge>
                    <a 
                      href={`https://ollama.com/library/${model.name.split(':')[0]}`} 
                      target="_blank" 
                      rel="noreferrer"
                      className="text-[9px] font-bold text-muted-foreground hover:text-primary flex items-center gap-1 transition-colors"
                    >
                      View on Ollama <ExternalLink size={10} />
                    </a>
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
        </TabsContent>

        <TabsContent value="requests" className="m-0">
          {requests.length === 0 ? (
            <div className="h-[400px] flex flex-col items-center justify-center bg-card border border-dashed border-border rounded-xl opacity-50">
               <CheckCircle2 size={40} className="mb-4 text-emerald-500" />
               <p className="text-sm font-black uppercase tracking-widest">No Pending Actions</p>
               <p className="text-[10px] font-bold text-muted-foreground mt-1">All cluster model operations are up to date</p>
            </div>
          ) : (
            <div className="space-y-3">
              {requests.map(req => (
                <Card key={req.id} className="bg-card border-border/50">
                  <CardContent className="p-4">
                    <div className="flex items-center gap-4">
                      <div className={`p-2.5 rounded-xl ${
                        req.type === 'pull' ? 'bg-blue-500/15 text-blue-400' :
                        req.type === 'delete' ? 'bg-red-500/15 text-red-400' :
                        'bg-amber-500/15 text-amber-400'
                      }`}>
                        {req.type === 'pull' ? <Download size={20} /> : 
                         req.type === 'delete' ? <Trash2 size={20} /> : <Box size={20} />}
                      </div>
                      
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2 mb-0.5">
                          <span className="text-[10px] font-black uppercase tracking-widest text-muted-foreground">
                            Manual {req.type} Approval
                          </span>
                          <Badge variant="outline" className="text-[8px] font-black h-4 px-1.5 bg-amber-500/10 text-amber-500 border-amber-500/20">
                            PENDING
                          </Badge>
                        </div>
                        <h3 className="text-sm font-black font-mono truncate">{req.model}</h3>
                        <p className="text-[10px] text-muted-foreground flex items-center gap-1.5 mt-1">
                          <Clock size={10} /> Requested {new Date(req.requested_at).toLocaleString()} 
                          · <Database size={10} /> {req.node_id ? `Target: ${req.node_id}` : 'Cluster-wide'}
                        </p>
                      </div>

                      <div className="flex items-center gap-2 shrink-0">
                         <Button 
                          variant="outline" 
                          size="sm" 
                          className="h-9 px-4 text-[10px] font-black uppercase tracking-widest hover:bg-red-500/10 hover:text-red-400 hover:border-red-500/30"
                          onClick={() => handleDecline(req.id)}
                         >
                           <XCircle size={14} className="mr-2" /> Decline
                         </Button>
                         <Button 
                          size="sm" 
                          className="h-9 px-4 text-[10px] font-black uppercase tracking-widest shadow-lg shadow-emerald-500/20"
                          onClick={() => handleApprove(req.id)}
                         >
                           <CheckCircle2 size={14} className="mr-2" /> Approve Action
                         </Button>
                      </div>
                    </div>
                  </CardContent>
                </Card>
              ))}
            </div>
          )}

          {/* Policy Note */}
          <div className="mt-8 p-6 bg-amber-500/5 border border-amber-500/20 rounded-xl flex gap-4">
             <ShieldAlert className="text-amber-500 shrink-0" size={24} />
             <div>
                <p className="text-xs font-black uppercase tracking-widest text-amber-500">Security Policy Active</p>
                <p className="text-[11px] text-muted-foreground mt-1 leading-relaxed">
                  Model approvals are required to prevent resource exhaustion and ensure cluster stability. 
                  Large models (e.g. 70B+) can significantly impact node performance and storage. 
                  Approving a pull will trigger the download only on the requested nodes.
                </p>
             </div>
          </div>
        </TabsContent>
      </Tabs>
    </div>
  );
};
