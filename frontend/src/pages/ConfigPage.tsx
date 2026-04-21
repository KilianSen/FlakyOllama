import React, { useState, useEffect } from 'react';
import { Save, RefreshCw, Shield, Zap, BarChart2, Clock } from 'lucide-react';
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from '@/components/ui/accordion';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui/switch';
import { Slider } from '@/components/ui/slider';
import { Skeleton } from '@/components/ui/skeleton';
import { Separator } from '@/components/ui/separator';
import { Badge } from '@/components/ui/badge';
import { toast } from 'sonner';
import { api, type Config } from '../api';

function FieldRow({
  label,
  description,
  children,
}: {
  label: string;
  description?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex items-start justify-between gap-6 py-3">
      <div className="flex-1 min-w-0">
        <p className="text-xs font-bold">{label}</p>
        {description && <p className="text-[10px] text-muted-foreground mt-0.5 leading-relaxed">{description}</p>}
      </div>
      <div className="shrink-0 w-40">{children}</div>
    </div>
  );
}

function NumberField({
  value,
  onChange,
  step = 1,
  min,
  max,
}: {
  value: number;
  onChange: (v: number) => void;
  step?: number;
  min?: number;
  max?: number;
}) {
  return (
    <Input
      type="number"
      value={value}
      step={step}
      min={min}
      max={max}
      onChange={e => {
        const v = step < 1 ? parseFloat(e.target.value) : parseInt(e.target.value);
        if (!isNaN(v)) onChange(v);
      }}
      className="h-8 bg-muted/50 border-border/50 text-xs font-mono text-right"
    />
  );
}

export const ConfigPage: React.FC = () => {
  const [config, setConfig] = useState<Config | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const [saving, setSaving] = useState(false);
  const [dirty, setDirty] = useState(false);

  // Connection settings (localStorage)
  const [localUrl, setLocalUrl] = useState(localStorage.getItem('BALANCER_URL') || '');
  const [localToken, setLocalToken] = useState(localStorage.getItem('BALANCER_TOKEN') || '');
  const [connDirty, setConnDirty] = useState(false);

  const saveConnection = () => {
    localStorage.setItem('BALANCER_URL', localUrl);
    localStorage.setItem('BALANCER_TOKEN', localToken);
    setConnDirty(false);
    toast.success('Connection settings saved. Refresh required.');
  };

  const load = () => {
    setLoading(true);
    setError(false);
    api.getConfig()
      .then(cfg => { setConfig(cfg); setDirty(false); })
      .catch(() => {
        toast.error('Failed to load configuration');
        setError(true);
      })
      .finally(() => setLoading(false));
  };

  useEffect(() => { load(); }, []);

  const set = (field: string, value: unknown) => {
    if (!config) return;
    setDirty(true);
    if (field.startsWith('weights.')) {
      setConfig({ ...config, weights: { ...config.weights, [field.slice(8)]: value } });
    } else if (field.startsWith('circuit_breaker.')) {
      setConfig({ ...config, circuit_breaker: { ...config.circuit_breaker, [field.slice(16)]: value } });
    } else {
      setConfig({ ...config, [field]: value });
    }
  };

  const save = async () => {
    if (!config) return;
    setSaving(true);
    try {
      await api.updateConfig(config);
      toast.success('Configuration applied');
      setDirty(false);
    } catch (err: any) {
      toast.error(err.message || 'Failed to save configuration');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="p-6 space-y-6 max-w-3xl">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-sm font-black uppercase tracking-widest">Cluster Configuration</h2>
          <p className="text-[11px] text-muted-foreground mt-0.5">Live runtime parameters — changes apply immediately</p>
        </div>
        <div className="flex items-center gap-2">
          {dirty && <Badge className="text-[9px] font-black bg-amber-500/15 text-amber-400 border-amber-500/30">Unsaved changes</Badge>}
          <Button variant="outline" size="sm" className="h-8 text-xs font-bold gap-1.5" onClick={load} disabled={loading}>
            <RefreshCw size={12} className={loading ? 'animate-spin' : ''} /> Reload
          </Button>
          <Button size="sm" className="h-8 text-xs font-black uppercase tracking-widest gap-1.5 shadow-lg shadow-primary/20" onClick={save} disabled={saving || !dirty || error}>
            {saving ? <RefreshCw size={12} className="animate-spin" /> : <Save size={12} />}
            Apply
          </Button>
        </div>
      </div>

      <Accordion type="multiple" defaultValue={['conn', 'hedging', 'routing', 'circuit', 'limits']} className="space-y-3">

        {/* Connection */}
        <AccordionItem value="conn" className="bg-card border border-border/50 rounded-xl overflow-hidden px-5">
          <AccordionTrigger className="text-xs font-black uppercase tracking-widest hover:no-underline py-4 gap-3">
            <div className="flex items-center gap-2">
              <Shield size={14} className="text-emerald-400" />
              Connection & Auth
            </div>
          </AccordionTrigger>
          <AccordionContent className="pb-4">
            <p className="text-[10px] text-muted-foreground mb-4 leading-relaxed">
              Frontend connection parameters. These are stored locally in your browser. Leave empty to use defaults from environment.
            </p>
            <Separator className="mb-4 bg-border/50" />
            <FieldRow label="Balancer URL" description="Base URL of the FlakyOllama Balancer (e.g. http://localhost:8080)">
              <Input
                value={localUrl}
                onChange={e => { setLocalUrl(e.target.value); setConnDirty(true); }}
                placeholder="Default (Relative)"
                className="h-8 bg-muted/50 border-border/50 text-xs font-mono"
              />
            </FieldRow>
            <FieldRow label="API Token" description="Bearer token for cluster authentication">
              <Input
                type="password"
                value={localToken}
                onChange={e => { setLocalToken(e.target.value); setConnDirty(true); }}
                placeholder="Default from ENV"
                className="h-8 bg-muted/50 border-border/50 text-xs font-mono"
              />
            </FieldRow>
            <div className="flex justify-end mt-2">
              <Button
                size="sm"
                variant="secondary"
                className="h-7 text-[10px] font-black uppercase"
                disabled={!connDirty}
                onClick={saveConnection}
              >
                Save Connection Settings
              </Button>
            </div>
          </AccordionContent>
        </AccordionItem>

        {loading ? (
          <div className="space-y-3 mt-3">
            {Array.from({ length: 4 }).map((_, i) => <Skeleton key={i} className="h-16 w-full rounded-xl" />)}
          </div>
        ) : error ? (
          <div className="p-12 text-center bg-card border border-dashed border-border rounded-xl">
            <p className="text-xs font-bold text-muted-foreground uppercase tracking-widest">Failed to connect to cluster</p>
            <p className="text-[10px] text-muted-foreground/60 mt-2">Check your connection settings above and retry</p>
            <Button variant="outline" size="sm" className="mt-4 h-8 text-xs font-bold" onClick={load}>
              Retry Connection
            </Button>
          </div>
        ) : config && (
          <>
            {/* Hedging */}
            <AccordionItem value="hedging" className="bg-card border border-border/50 rounded-xl overflow-hidden px-5">
          <AccordionTrigger className="text-xs font-black uppercase tracking-widest hover:no-underline py-4 gap-3">
            <div className="flex items-center gap-2">
              <Zap size={14} className="text-primary" />
              Request Hedging
            </div>
          </AccordionTrigger>
          <AccordionContent className="pb-4">
            <p className="text-[10px] text-muted-foreground mb-4 leading-relaxed">
              Duplicate delayed requests on an alternate node to reduce tail latency. The percentile threshold determines when a request is considered stale enough to hedge.
            </p>
            <Separator className="mb-4 bg-border/50" />
            <FieldRow label="Enable Hedging" description="Activate request duplication on slow nodes">
              <div className="flex justify-end">
                <Switch
                  checked={config.enable_hedging}
                  onCheckedChange={v => set('enable_hedging', v)}
                />
              </div>
            </FieldRow>
            <FieldRow label="Percentile Threshold" description="Latency percentile above which hedging triggers (0.0–1.0)">
              <div className="space-y-2">
                <div className="flex justify-between text-[9px] font-bold text-muted-foreground">
                  <span>0.0</span>
                  <span className="text-foreground font-black">{config.hedging_percentile.toFixed(2)}</span>
                  <span>1.0</span>
                </div>
                <Slider
                  disabled={!config.enable_hedging}
                  value={[config.hedging_percentile]}
                  onValueChange={([v]) => set('hedging_percentile', v)}
                  min={0} max={1} step={0.01}
                  className="w-full"
                />
              </div>
            </FieldRow>
          </AccordionContent>
        </AccordionItem>

        {/* Routing Weights */}
        <AccordionItem value="routing" className="bg-card border border-border/50 rounded-xl overflow-hidden px-5">
          <AccordionTrigger className="text-xs font-black uppercase tracking-widest hover:no-underline py-4 gap-3">
            <div className="flex items-center gap-2">
              <BarChart2 size={14} className="text-purple-400" />
              Routing Heuristics
            </div>
          </AccordionTrigger>
          <AccordionContent className="pb-4">
            <p className="text-[10px] text-muted-foreground mb-4 leading-relaxed">
              Scoring weights used by the routing algorithm. Higher values increase a factor's influence on node selection.
            </p>
            <Separator className="mb-4 bg-border/50" />
            {[
              { key: 'weights.cpu_load_weight', label: 'CPU Load Weight', desc: 'Penalizes high CPU utilization' },
              { key: 'weights.latency_weight', label: 'Latency Weight', desc: 'Favors historically faster nodes' },
              { key: 'weights.success_rate_weight', label: 'Success Rate Weight', desc: 'Prioritizes reliable nodes' },
              { key: 'weights.loaded_model_bonus', label: 'Loaded Model Bonus', desc: 'Rewards nodes with the model in VRAM' },
              { key: 'weights.workload_penalty', label: 'Workload Penalty', desc: 'Penalizes nodes with many active requests' },
              { key: 'weights.local_model_bonus', label: 'Local Model Bonus', desc: 'Rewards nodes with model on disk' },
            ].map(({ key, label, desc }) => {
              const val = key.startsWith('weights.') ? config.weights[key.slice(8) as keyof typeof config.weights] : 0;
              return (
                <FieldRow key={key} label={label} description={desc}>
                  <NumberField value={val} onChange={v => set(key, v)} step={0.1} min={0} />
                </FieldRow>
              );
            })}
          </AccordionContent>
        </AccordionItem>

        {/* Circuit Breaker */}
        <AccordionItem value="circuit" className="bg-card border border-border/50 rounded-xl overflow-hidden px-5">
          <AccordionTrigger className="text-xs font-black uppercase tracking-widest hover:no-underline py-4 gap-3">
            <div className="flex items-center gap-2">
              <Shield size={14} className="text-red-400" />
              Circuit Breaker
            </div>
          </AccordionTrigger>
          <AccordionContent className="pb-4">
            <p className="text-[10px] text-muted-foreground mb-4 leading-relaxed">
              Automatically isolates unhealthy nodes after repeated failures, preventing cascading errors across the fleet.
            </p>
            <Separator className="mb-4 bg-border/50" />
            <FieldRow label="Error Threshold" description="Consecutive errors before a node is put in cooloff">
              <NumberField value={config.circuit_breaker.error_threshold} onChange={v => set('circuit_breaker.error_threshold', v)} min={1} />
            </FieldRow>
            <FieldRow label="Cooloff Duration" description="Seconds a node stays in cooloff before retrying">
              <NumberField value={config.circuit_breaker.cooloff_sec} onChange={v => set('circuit_breaker.cooloff_sec', v)} min={1} />
            </FieldRow>
          </AccordionContent>
        </AccordionItem>

        {/* System Limits */}
        <AccordionItem value="limits" className="bg-card border border-border/50 rounded-xl overflow-hidden px-5">
          <AccordionTrigger className="text-xs font-black uppercase tracking-widest hover:no-underline py-4 gap-3">
            <div className="flex items-center gap-2">
              <Clock size={14} className="text-teal-400" />
              System Limits & Timers
            </div>
          </AccordionTrigger>
          <AccordionContent className="pb-4">
            <p className="text-[10px] text-muted-foreground mb-4 leading-relaxed">
              Queue depths, timeouts, and polling intervals that control balancer throughput and responsiveness.
            </p>
            <Separator className="mb-4 bg-border/50" />
            <FieldRow label="Max Queue Depth" description="Maximum number of requests held in queue">
              <NumberField value={config.max_queue_depth} onChange={v => set('max_queue_depth', v)} min={1} />
            </FieldRow>
            <FieldRow label="Stall Timeout (s)" description="Seconds before a stalled request is retried or failed">
              <NumberField value={config.stall_timeout_sec} onChange={v => set('stall_timeout_sec', v)} min={1} />
            </FieldRow>
            <FieldRow label="Keep Alive Duration (s)" description="Seconds a model stays resident in VRAM when idle">
              <NumberField value={config.keep_alive_duration_sec} onChange={v => set('keep_alive_duration_sec', v)} min={0} />
            </FieldRow>
            <FieldRow label="Stale Threshold" description="Queue depth at which a model is replicated to another node">
              <NumberField value={config.stale_threshold} onChange={v => set('stale_threshold', v)} min={1} />
            </FieldRow>
            <FieldRow label="Poll Interval (ms)" description="How frequently agents report telemetry to the balancer">
              <NumberField value={config.poll_interval_ms} onChange={v => set('poll_interval_ms', v)} min={50} />
            </FieldRow>
          </AccordionContent>
        </AccordionItem>
          </>
        )}

      </Accordion>
    </div>
  );
};
