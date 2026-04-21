import React, { useState, useMemo } from 'react';
import { Search, CloudDownload, RefreshCw, XCircle, Trash2, Box, ChevronRight } from 'lucide-react';
import { Card } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { Progress } from '@/components/ui/progress';
import { toast } from 'sonner';
import sdk from '../api';
import type { NodeStatus } from '../api';
import { useCluster } from '../ClusterContext';
import {
  computeRoutability, inferCapabilities, inferSDKCompat,
  CAPABILITY_LABELS, LATENCY_HINTS, formatBytes, parseModelTag,
} from '../lib/modelUtils';

// ─── Model Detail Panel ───────────────────────────────────────────────────────
function ModelDetailPanel({ modelName, onClose, onAction }: {
  modelName: string;
  onClose: () => void;
  onAction: (action: () => Promise<any>, msg: string) => void;
}) {
  const { status } = useCluster();
  if (!status) return null;

  const r    = computeRoutability(modelName, status);
  const caps = inferCapabilities(modelName);
  const compat = inferSDKCompat(caps);
  const { family, variant } = parseModelTag(modelName);
  const hint = LATENCY_HINTS[r.latencyHint];

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-end" onClick={onClose}>
      <div
        className="w-[400px] h-full bg-card border-l border-border/50 shadow-2xl flex flex-col overflow-hidden"
        onClick={e => e.stopPropagation()}
      >
        {/* Header */}
        <div className="p-5 border-b border-border/50 flex items-start justify-between">
          <div>
            <div className="flex items-center gap-2 mb-1">
              <span className="font-black text-sm font-mono">{family}</span>
              <Badge variant="outline" className="text-[9px] font-black h-5">:{variant}</Badge>
            </div>
            <p className={`text-[10px] font-black uppercase ${hint.color}`}>{hint.label}</p>
          </div>
          <Button variant="ghost" size="icon" className="h-7 w-7 text-muted-foreground" onClick={onClose}>
            <XCircle size={14} />
          </Button>
        </div>

        <div className="flex-1 overflow-y-auto p-5 space-y-6">
          {/* Capabilities */}
          <div>
            <p className="text-[9px] font-black uppercase tracking-widest text-muted-foreground mb-2">Capabilities</p>
            <div className="flex flex-wrap gap-1.5">
              {caps.map(c => {
                const meta = CAPABILITY_LABELS[c];
                return (
                  <Badge key={c} className={`text-[9px] font-black h-5 px-2 border ${meta.color}`}>
                    {meta.icon} {meta.label}
                  </Badge>
                );
              })}
            </div>
          </div>

          {/* SDK compatibility matrix */}
          <div>
            <p className="text-[9px] font-black uppercase tracking-widest text-muted-foreground mb-3">SDK Compatibility</p>
            <div className="space-y-2">
              {[
                {
                  label: 'Ollama SDK — Native',
                  sub: 'ollama.generate() · ollama.chat()',
                  ok: compat.ollamaNative,
                },
                {
                  label: 'Ollama SDK — Embeddings',
                  sub: 'ollama.embeddings()',
                  ok: compat.ollamaEmbed,
                },
                {
                  label: 'OpenAI SDK — Chat',
                  sub: '/v1/chat/completions · streaming',
                  ok: compat.openAIChat,
                  warn: compat.openAIWarning,
                },
                {
                  label: 'OpenAI SDK — Embeddings',
                  sub: '/v1/embeddings',
                  ok: compat.openAIEmbed,
                },
              ].map(row => (
                <div key={row.label} className={`flex items-start gap-3 p-2.5 rounded-lg border ${
                  row.ok ? 'bg-muted/20 border-border/30' : 'bg-muted/5 border-border/10 opacity-40'
                }`}>
                  <span className="text-base mt-0.5">{row.ok ? '✓' : '✗'}</span>
                  <div className="min-w-0">
                    <p className={`text-[10px] font-black ${row.ok ? 'text-foreground' : 'text-muted-foreground'}`}>
                      {row.label}
                    </p>
                    <p className="text-[9px] text-muted-foreground font-mono">{row.sub}</p>
                    {row.warn && (
                      <p className="text-[9px] text-amber-400 mt-1 leading-relaxed">⚠ {row.warn}</p>
                    )}
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* Node routing matrix */}
          <div>
            <p className="text-[9px] font-black uppercase tracking-widest text-muted-foreground mb-3">Node Routing Matrix</p>
            <div className="space-y-2">
              {r.residency.map(res => {
                const vramPct = res.node.vram_total > 0
                  ? (res.node.vram_used / res.node.vram_total) * 100 : 0;
                const thermalColors = {
                  hot:  { bar: 'bg-emerald-400', text: 'text-emerald-400', bg: 'border-emerald-500/20 bg-emerald-500/5' },
                  warm: { bar: 'bg-amber-400',   text: 'text-amber-400',   bg: 'border-amber-500/20 bg-amber-500/5'   },
                  cold: { bar: 'bg-muted',        text: 'text-muted-foreground', bg: 'border-border/20 bg-muted/5' },
                };
                const c = thermalColors[res.thermal];

                return (
                  <div key={res.node.id} className={`p-3 rounded-lg border ${c.bg}`}>
                    <div className="flex items-center justify-between mb-2">
                      <div className="flex items-center gap-2">
                        <span className={`text-[10px] font-black ${c.text}`}>
                          {res.thermal === 'hot' ? '⚡' : res.thermal === 'warm' ? '💾' : '○'}
                        </span>
                        <span className="text-xs font-black">{res.node.id}</span>
                        <Badge variant="outline" className="text-[8px] font-bold h-4">{res.node.tier}</Badge>
                      </div>
                      <span className={`text-[9px] font-black uppercase ${c.text}`}>
                        {res.thermal === 'hot' ? 'In VRAM' : res.thermal === 'warm' ? `On Disk${res.size ? ` · ${formatBytes(res.size)}` : ''}` : 'Not Present'}
                      </span>
                    </div>
                    {res.node.has_gpu && (
                      <div>
                        <div className="flex justify-between text-[8px] text-muted-foreground font-bold mb-1">
                          <span>VRAM</span>
                          <span>{formatBytes(res.node.vram_used)} / {formatBytes(res.node.vram_total)}</span>
                        </div>
                        <Progress value={vramPct} className="h-1" />
                      </div>
                    )}
                    {res.thermal !== 'cold' && (
                      <div className="flex gap-1 mt-2">
                        {res.thermal === 'hot' && (
                          <Button
                            size="sm" variant="ghost"
                            className="h-6 text-[9px] font-black uppercase text-amber-400 hover:bg-amber-500/10 px-2"
                            onClick={() => onAction(
                              () => sdk.unloadModel(modelName, res.node.id),
                              `Evicted ${modelName} from ${res.node.id}`
                            )}
                          >
                            Evict from VRAM
                          </Button>
                        )}
                        <Button
                          size="sm" variant="ghost"
                          className="h-6 text-[9px] font-black uppercase text-destructive hover:bg-destructive/10 px-2"
                          onClick={() => onAction(
                            () => sdk.deleteModel(modelName),
                            `Deleted ${modelName} from fleet`
                          )}
                        >
                          Delete
                        </Button>
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          </div>
        </div>

        {/* Footer actions */}
        <div className="p-4 border-t border-border/50 flex gap-2">
          <Button
            className="flex-1 h-9 text-xs font-black uppercase tracking-widest shadow-lg shadow-primary/20"
            onClick={() => onAction(() => sdk.pullModel(modelName), `Syncing ${modelName} to fleet`)}
          >
            <CloudDownload size={13} className="mr-2" /> Sync to Fleet
          </Button>
          <Button
            variant="destructive" size="icon" className="h-9 w-9"
            onClick={() => { onClose(); onAction(() => sdk.deleteModel(modelName), `Deleted ${modelName}`); }}
          >
            <Trash2 size={13} />
          </Button>
        </div>
      </div>
    </div>
  );
}

// ─── Main Registry Page ───────────────────────────────────────────────────────
export const RegistryPage: React.FC = () => {
  const { status, refresh: onRefresh } = useCluster();
  if (!status) return null;

  const [search, setSearch]           = useState('');
  const [pullOpen, setPullOpen]       = useState(false);
  const [selectedModel, setSelectedModel] = useState<string | null>(null);
  const [sortBy, setSortBy]           = useState<'name' | 'hot' | 'nodes'>('hot');

  const handleAction = async (action: () => Promise<any>, msg: string) => {
    const tid = toast.loading('Orchestrating...');
    try {
      const res = await action();
      if (res?.job_id) {
        toast.info('Job started', { id: tid });
        await sdk.waitForJob(res.job_id);
        toast.success(msg, { id: tid });
      } else {
        toast.success(msg, { id: tid });
      }
      onRefresh();
    } catch (err: any) {
      toast.error(err.message || 'Action failed', { id: tid });
    }
  };

  const allNodes = Object.values(status.nodes) as NodeStatus[];

  const modelData = useMemo(() => {
    return (status.all_models || []).map(name => ({
      name,
      r: computeRoutability(name, status),
      caps: inferCapabilities(name),
    }));
  }, [status]);

  const sorted = useMemo(() => [...modelData].sort((a, b) => {
    if (sortBy === 'hot')   return b.r.hotCount - a.r.hotCount || b.r.warmCount - a.r.warmCount;
    if (sortBy === 'nodes') return (b.r.hotCount + b.r.warmCount) - (a.r.hotCount + a.r.warmCount);
    return a.name.localeCompare(b.name);
  }), [modelData, sortBy]);

  const filtered = sorted.filter(m => m.name.toLowerCase().includes(search.toLowerCase()));

  // Cluster-wide summary
  const hotModels  = modelData.filter(m => m.r.hotCount > 0).length;
  const warmModels = modelData.filter(m => m.r.hotCount === 0 && m.r.warmCount > 0).length;
  const coldModels = modelData.filter(m => m.r.hotCount + m.r.warmCount === 0).length;

  return (
    <TooltipProvider>
      {selectedModel && (
        <ModelDetailPanel
          modelName={selectedModel}
          onClose={() => setSelectedModel(null)}
          onAction={handleAction}
        />
      )}

      <div className="p-6 space-y-4">
        {/* Header */}
        <div className="flex items-center justify-between">
          <div>
            <h2 className="text-sm font-black uppercase tracking-widest">Model Registry</h2>
            <div className="flex items-center gap-3 mt-1">
              <span className="text-[10px] text-emerald-400 font-black">{hotModels} hot</span>
              <span className="text-muted-foreground/30">·</span>
              <span className="text-[10px] text-amber-400 font-black">{warmModels} warm</span>
              <span className="text-muted-foreground/30">·</span>
              <span className="text-[10px] text-muted-foreground font-black">{coldModels} unavailable</span>
              <span className="text-muted-foreground/30">·</span>
              <span className="text-[10px] text-muted-foreground font-bold">{allNodes.length} nodes</span>
            </div>
          </div>
          <div className="flex items-center gap-2">
            {/* Sort */}
            <div className="flex items-center gap-1 text-[9px] font-black uppercase tracking-widest text-muted-foreground">
              {(['hot', 'nodes', 'name'] as const).map(s => (
                <button
                  key={s}
                  onClick={() => setSortBy(s)}
                  className={`px-2 py-1 rounded transition-colors ${sortBy === s ? 'bg-primary/20 text-primary' : 'hover:text-foreground'}`}
                >
                  {s}
                </button>
              ))}
            </div>
            <div className="relative">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" size={12} />
              <Input
                placeholder="Filter models..."
                className="pl-9 h-8 w-52 bg-muted/50 border-border/50 text-xs font-bold"
                value={search}
                onChange={e => setSearch(e.target.value)}
              />
            </div>
            <Dialog open={pullOpen} onOpenChange={setPullOpen}>
              <Button size="sm" className="h-8 text-xs font-black uppercase tracking-widest gap-2" onClick={() => setPullOpen(true)}>
                <CloudDownload size={13} /> Pull to Fleet
              </Button>
              <DialogContent className="bg-card border-border/50">
                <DialogHeader>
                  <DialogTitle className="font-black uppercase tracking-tight">Fleet Pull</DialogTitle>
                  <DialogDescription className="text-xs font-bold text-muted-foreground">
                    Broadcast a model pull to all compute nodes
                  </DialogDescription>
                </DialogHeader>
                <form onSubmit={e => {
                  e.preventDefault();
                  const tag = new FormData(e.currentTarget).get('tag') as string;
                  setPullOpen(false);
                  handleAction(() => sdk.pullModel(tag), `Syncing ${tag} to fleet`);
                }} className="space-y-4 pt-2">
                  <Input name="tag" placeholder="e.g. llama3.2:8b" className="font-bold bg-muted/50 border-border/50 font-mono" required autoFocus />
                  <DialogFooter>
                    <Button type="submit" className="w-full font-black uppercase text-xs tracking-widest">
                      Orchestrate Global Sync
                    </Button>
                  </DialogFooter>
                </form>
              </DialogContent>
            </Dialog>
          </div>
        </div>

        {/* Table */}
        <Card className="bg-card border-border/50 overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow className="border-border/50 hover:bg-transparent">
                <TableHead className="text-[9px] font-black uppercase tracking-widest text-muted-foreground w-[280px]">Model</TableHead>
                <TableHead className="text-[9px] font-black uppercase tracking-widest text-muted-foreground">Capabilities</TableHead>
                <TableHead className="text-[9px] font-black uppercase tracking-widest text-muted-foreground">SDK Compat</TableHead>
                <TableHead className="text-[9px] font-black uppercase tracking-widest text-muted-foreground">Node Coverage</TableHead>
                <TableHead className="text-right text-[9px] font-black uppercase tracking-widest text-muted-foreground">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filtered.length === 0 && (
                <TableRow>
                  <TableCell colSpan={5} className="text-center py-16 text-muted-foreground">
                    <Box size={32} className="mx-auto mb-3 opacity-30" />
                    <p className="text-xs font-bold uppercase tracking-widest opacity-50">No models found</p>
                  </TableCell>
                </TableRow>
              )}
              {filtered.map(({ name, r, caps }) => {
                const compat = inferSDKCompat(caps);
                const hint   = LATENCY_HINTS[r.latencyHint];
                const { family, variant } = parseModelTag(name);

                return (
                  <TableRow
                    key={name}
                    className="group border-border/30 hover:bg-muted/10 transition-colors cursor-pointer"
                    onClick={() => setSelectedModel(name)}
                  >
                    {/* Model name */}
                    <TableCell className="py-3">
                      <div className="flex items-center gap-2">
                        <span className={`w-1.5 h-1.5 rounded-full shrink-0 ${
                          r.hotCount  > 0 ? 'bg-emerald-400' :
                          r.warmCount > 0 ? 'bg-amber-400' : 'bg-muted-foreground/30'
                        }`} />
                        <div>
                          <div className="flex items-center gap-1.5">
                            <span className="text-xs font-black font-mono">{family}</span>
                            <span className="text-[9px] text-muted-foreground font-mono">:{variant}</span>
                          </div>
                          <div className="flex items-center gap-2 mt-0.5">
                            <span className={`text-[9px] font-black ${hint.color}`}>{hint.label}</span>
                            {r.syncing && (
                              <Badge className="text-[8px] h-4 px-1 font-black bg-primary/10 text-primary border-primary/20 animate-pulse">
                                <RefreshCw size={7} className="animate-spin mr-1" /> Syncing
                              </Badge>
                            )}
                          </div>
                        </div>
                      </div>
                    </TableCell>

                    {/* Capabilities */}
                    <TableCell>
                      <div className="flex flex-wrap gap-1">
                        {caps.map(c => {
                          const meta = CAPABILITY_LABELS[c];
                          return (
                            <Badge key={c} className={`text-[8px] font-black h-4 px-1.5 border ${meta.color}`}>
                              {meta.icon} {meta.label}
                            </Badge>
                          );
                        })}
                      </div>
                    </TableCell>

                    {/* SDK compat */}
                    <TableCell>
                      <div className="flex items-center gap-1.5 text-[9px] font-black">
                        <Tooltip>
                          <TooltipTrigger>
                            <span className={compat.ollamaNative ? 'text-emerald-400' : 'text-muted-foreground/30'}>
                              Ollama
                            </span>
                          </TooltipTrigger>
                          <TooltipContent className="text-[10px] font-bold">
                            {compat.ollamaNative ? '✓ Ollama SDK supported' : '✗ Not supported'}
                          </TooltipContent>
                        </Tooltip>
                        <span className="text-muted-foreground/30">·</span>
                        <Tooltip>
                          <TooltipTrigger>
                            <span className={compat.openAIChat ? 'text-emerald-400' : compat.openAIEmbed ? 'text-amber-400' : 'text-muted-foreground/30'}>
                              OpenAI
                            </span>
                          </TooltipTrigger>
                          <TooltipContent className="text-[10px] font-bold max-w-[200px]">
                            {compat.openAIChat
                              ? `✓ /v1/chat/completions${compat.openAIWarning ? '\n⚠ ' + compat.openAIWarning : ''}`
                              : compat.openAIEmbed
                                ? '✓ /v1/embeddings only'
                                : '✗ Not supported'}
                          </TooltipContent>
                        </Tooltip>
                      </div>
                    </TableCell>

                    {/* Node coverage */}
                    <TableCell>
                      <div className="flex items-center gap-1.5">
                        {r.residency.map(res => {
                          const dot = res.thermal === 'hot'  ? 'bg-emerald-400' :
                                      res.thermal === 'warm' ? 'bg-amber-400' :
                                                               'bg-muted-foreground/20';
                          return (
                            <Tooltip key={res.node.id}>
                              <TooltipTrigger>
                                <div className={`w-2 h-2 rounded-full ${dot}`} />
                              </TooltipTrigger>
                              <TooltipContent className="text-[10px] font-bold">
                                {res.node.id} — {res.thermal === 'hot' ? '⚡ Hot' : res.thermal === 'warm' ? '💾 Warm' : '○ Cold'}
                              </TooltipContent>
                            </Tooltip>
                          );
                        })}
                        <span className="text-[9px] font-bold text-muted-foreground ml-1">
                          {r.hotCount + r.warmCount}/{r.totalNodes}
                        </span>
                      </div>
                    </TableCell>

                    {/* Actions */}
                    <TableCell className="text-right" onClick={e => e.stopPropagation()}>
                      <div className="flex justify-end gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Button
                              disabled={r.syncing}
                              variant="ghost" size="icon"
                              className="h-7 w-7 text-amber-400 hover:bg-amber-500/10"
                              onClick={() => handleAction(() => sdk.unloadModel(name), `Evicted ${name}`)}
                            >
                              <XCircle size={13} />
                            </Button>
                          </TooltipTrigger>
                          <TooltipContent className="text-[10px] font-bold">Evict from VRAM</TooltipContent>
                        </Tooltip>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Button
                              disabled={r.syncing}
                              variant="ghost" size="icon"
                              className="h-7 w-7 text-destructive hover:bg-destructive/10"
                              onClick={() => handleAction(() => sdk.deleteModel(name), `Deleted ${name}`)}
                            >
                              <Trash2 size={13} />
                            </Button>
                          </TooltipTrigger>
                          <TooltipContent className="text-[10px] font-bold">Delete from Fleet</TooltipContent>
                        </Tooltip>
                        <Button
                          variant="ghost" size="icon"
                          className="h-7 w-7 text-muted-foreground hover:text-foreground"
                          onClick={() => setSelectedModel(name)}
                        >
                          <ChevronRight size={13} />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </Card>
      </div>
    </TooltipProvider>
  );
};
