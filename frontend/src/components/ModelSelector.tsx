import React from 'react';
import { AlertTriangle } from 'lucide-react';
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select';
import { Badge } from '@/components/ui/badge';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { useCluster } from '../ClusterContext';
import {
  computeRoutability, inferCapabilities, inferSDKCompat,
  CAPABILITY_LABELS, LATENCY_HINTS, deriveVirtualModelMeta, formatBytes,
} from '../lib/modelUtils';

interface ModelSelectorProps {
  value: string;
  onChange: (v: string) => void;
  sdkMode?: 'flakyollama' | 'ollama' | 'openai';
  className?: string;
  placeholder?: string;
}

export const ModelSelector: React.FC<ModelSelectorProps> = ({
  value, onChange, sdkMode = 'ollama', className, placeholder = 'Select model...',
}) => {
  const { status } = useCluster();
  const models = status?.all_models ?? [];

  return (
    <TooltipProvider>
      <div className="space-y-1.5">
        <Select value={value} onValueChange={onChange}>
          <SelectTrigger className={`bg-muted/50 border-border/50 font-bold text-xs h-9 ${className ?? ''}`}>
            <SelectValue placeholder={placeholder} />
          </SelectTrigger>
          <SelectContent className="max-h-80">
            {models.length === 0 ? (
              <SelectItem value="none" disabled className="text-xs">No models in cluster</SelectItem>
            ) : (
              models.map(m => {
                const r = status ? computeRoutability(m, status) : null;
                const hint = r ? LATENCY_HINTS[r.latencyHint] : null;
                return (
                  <SelectItem key={m} value={m} className="pr-2">
                    <div className="flex items-center gap-2 w-full min-w-0">
                      <span className="font-bold text-xs font-mono truncate flex-1">{m}</span>
                      {hint && (
                        <span className={`text-[9px] font-black shrink-0 ${hint.color}`}>
                          {hint.label}
                        </span>
                      )}
                    </div>
                  </SelectItem>
                );
              })
            )}
          </SelectContent>
        </Select>

        {/* Inline routability + compat panel for selected model */}
        {value && status && (() => {
          const r = computeRoutability(value, status);
          const isVirtual = !!status.virtual_models?.[value];
          const virtualCfg = status.virtual_models?.[value];
          const caps = isVirtual
            ? deriveVirtualModelMeta(virtualCfg!, Object.values(status.nodes)).capabilities
            : inferCapabilities(value);
          const compat = inferSDKCompat(caps);
          const hint = LATENCY_HINTS[r.latencyHint];
          const virtualMeta = isVirtual
            ? deriveVirtualModelMeta(virtualCfg!, Object.values(status.nodes))
            : null;

          const showOpenAIWarning = sdkMode === 'openai' && compat.openAIWarning;

          return (
            <div className="rounded-lg border border-border/40 bg-muted/20 p-3 space-y-2.5">
              {/* Routing summary */}
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <span className="text-[9px] font-black uppercase tracking-widest text-muted-foreground">Routing</span>
                  {isVirtual && (
                    <Badge variant="outline" className="text-[7px] h-3.5 px-1 border-violet-500/30 text-violet-400 uppercase font-black">Virtual</Badge>
                  )}
                </div>
                <div className="flex items-center gap-2">
                  <span className={`text-[10px] font-black ${hint.color}`}>{hint.label}</span>
                  {!isVirtual && r.hotCount > 0 && (
                    <span className="text-[9px] text-muted-foreground">{r.hotCount} hot</span>
                  )}
                  {!isVirtual && r.warmCount > 0 && (
                    <span className="text-[9px] text-muted-foreground">{r.warmCount} warm</span>
                  )}
                  {r.syncing && (
                    <Badge className="text-[8px] h-4 px-1.5 bg-primary/10 text-primary border-primary/20 animate-pulse">
                      Syncing
                    </Badge>
                  )}
                  {isVirtual && virtualMeta && virtualMeta.effectiveSizeBytes > 0 && (
                    <span className="text-[9px] text-muted-foreground">{formatBytes(virtualMeta.effectiveSizeBytes)}</span>
                  )}
                </div>
              </div>

              {/* Per-node status pills (physical only) */}
              {!isVirtual && (
                <div className="flex flex-wrap gap-1">
                  {r.residency.map(res => {
                    const colors = {
                      hot:  'bg-emerald-500/15 text-emerald-400 border-emerald-500/25',
                      warm: 'bg-amber-500/15 text-amber-400 border-amber-500/25',
                      cold: 'bg-muted/30 text-muted-foreground/40 border-border/20',
                    };
                    return (
                      <Tooltip key={res.node.id}>
                        <TooltipTrigger>
                          <Badge
                            className={`text-[8px] font-black h-5 px-2 border cursor-default ${colors[res.thermal]}`}
                          >
                            {res.thermal === 'hot' ? '⚡' : res.thermal === 'warm' ? '💾' : '○'}{' '}
                            {res.node.id.split('-').pop()}
                          </Badge>
                        </TooltipTrigger>
                        <TooltipContent className="text-[10px] font-bold space-y-0.5">
                          <p>{res.node.id}</p>
                          <p className="text-muted-foreground">
                            {res.thermal === 'hot'  ? '🔥 Active in VRAM'   :
                             res.thermal === 'warm' ? `💾 On disk${res.size ? ` · ${(res.size / 1e9).toFixed(1)} GB` : ''}` :
                             '○ Not present'}
                          </p>
                        </TooltipContent>
                      </Tooltip>
                    );
                  })}
                </div>
              )}

              {/* Virtual: per-target availability */}
              {isVirtual && virtualCfg && (
                <div className="flex flex-wrap gap-1">
                  {(virtualCfg.type === 'pipeline'
                    ? (virtualCfg.steps?.map(s => s.model) ?? virtualCfg.targets ?? [])
                    : (virtualCfg.targets ?? [])
                  ).map(target => {
                    const tr = computeRoutability(target, status);
                    const th = LATENCY_HINTS[tr.latencyHint];
                    const sz = virtualMeta?.targetSizes[target];
                    return (
                      <Tooltip key={target}>
                        <TooltipTrigger>
                          <Badge variant="outline" className={`text-[8px] font-black h-5 px-2 border cursor-default ${th.color} border-current/20`}>
                            {target}
                          </Badge>
                        </TooltipTrigger>
                        <TooltipContent className="text-[10px] font-bold space-y-0.5">
                          <p className="font-mono">{target}</p>
                          <p className={`${th.color}`}>{th.label}</p>
                          {sz && <p className="text-muted-foreground">{formatBytes(sz)} on disk</p>}
                        </TooltipContent>
                      </Tooltip>
                    );
                  })}
                </div>
              )}

              {/* Capabilities */}
              <div className="flex items-center gap-2 flex-wrap">
                <span className="text-[9px] font-black uppercase tracking-widest text-muted-foreground">Caps</span>
                {caps.map(c => {
                  const meta = CAPABILITY_LABELS[c];
                  return (
                    <Badge key={c} className={`text-[8px] font-black h-4 px-1.5 border ${meta.color}`}>
                      {meta.icon} {meta.label}
                    </Badge>
                  );
                })}
              </div>

              {/* SDK warnings */}
              {showOpenAIWarning && (
                <div className="flex items-start gap-1.5 text-amber-400 bg-amber-500/10 rounded-md px-2.5 py-2 border border-amber-500/20">
                  <AlertTriangle size={11} className="shrink-0 mt-0.5" />
                  <p className="text-[9px] font-bold leading-relaxed">{compat.openAIWarning}</p>
                </div>
              )}
              {!r.routable && !r.syncing && (
                <div className="flex items-start gap-1.5 text-red-400 bg-red-500/10 rounded-md px-2.5 py-2 border border-red-500/20">
                  <AlertTriangle size={11} className="shrink-0 mt-0.5" />
                  <p className="text-[9px] font-bold leading-relaxed">
                    {isVirtual
                      ? 'None of the backing models are available on any node'
                      : 'Model not present on any node — pull it first in the Registry'}
                  </p>
                </div>
              )}
            </div>
          );
        })()}
      </div>
    </TooltipProvider>
  );
};
