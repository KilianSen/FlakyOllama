import React, { useState, useEffect } from 'react';
import { Save, RefreshCw, Shield, Zap, BarChart2, Clock, Trash2, TrendingUp, Cpu, Layers, Plus } from 'lucide-react';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui/switch';
import { Slider } from '@/components/ui/slider';
import { Skeleton } from '@/components/ui/skeleton';
import { Separator } from '@/components/ui/separator';
import { Badge } from '@/components/ui/badge';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { toast } from 'sonner';
import { api, type Config } from '../api';
import { useCluster } from '../ClusterContext';

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
    <div className="flex flex-col gap-3 py-3 md:flex-row md:items-start md:justify-between md:gap-6">
      <div className="flex-1 min-w-0">
        <p className="text-sm font-semibold leading-tight">{label}</p>
        {description && <p className="mt-1 text-xs text-muted-foreground leading-relaxed">{description}</p>}
      </div>
      <div className="w-full shrink-0 md:w-56">{children}</div>
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
      className="h-9 bg-muted/50 border-border/50 text-sm font-mono text-right"
    />
  );
}

function FactorTable({ title, factors, onUpdate }: { title: string, factors: Record<string, number>, onUpdate: (f: Record<string, number>) => void }) {
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
         <p className="text-xs font-bold uppercase tracking-wide text-muted-foreground">{title}</p>
         <Button
           variant="outline" size="sm" className="h-8 text-xs font-semibold"
           onClick={() => {
             const m = prompt('Enter model name:');
             if (m) onUpdate({ ...factors, [m]: 1.0 });
           }}
         >+ Add</Button>
      </div>
      <div className="space-y-2">
        {Object.entries(factors).map(([model, factor]) => (
          <div key={model} className="grid grid-cols-1 gap-2 rounded-lg border border-border/40 bg-muted/20 p-3 sm:grid-cols-[minmax(0,1fr)_96px_32px] sm:items-center">
            <span className="truncate text-xs font-mono font-semibold">{model}</span>
            <Input
              type="number"
              className="h-8 w-full bg-background text-sm font-medium"
              value={factor}
              onChange={e => onUpdate({ ...factors, [model]: parseFloat(e.target.value) })}
            />
            <Button 
              variant="ghost" size="sm" className="h-8 w-8 p-0"
              onClick={() => {
                const next = { ...factors };
                delete next[model];
                onUpdate(next);
              }}
            ><Trash2 size={14} /></Button>
          </div>
        ))}
        {Object.keys(factors).length === 0 && (
          <p className="py-2 text-center text-xs italic text-muted-foreground/70">No custom factors defined</p>
        )}
      </div>
    </div>
  );
}

function VirtualModelTable({ 
    virtualModels, 
    onUpdate,
    availableModels 
  }: { 
    virtualModels: Record<string, any>, 
    onUpdate: (f: Record<string, any>) => void,
    availableModels: string[]
  }) {
    return (
      <div className="space-y-4">
        <div className="flex items-center justify-between">
           <p className="text-xs font-bold uppercase tracking-wide text-muted-foreground">Registered Virtual Models</p>
           <Button
             variant="outline" size="sm" className="h-8 text-[10px] font-black uppercase tracking-widest gap-2"
             onClick={() => {
               const m = prompt('Enter virtual model alias (e.g. auto-coder):');
               if (m) onUpdate({ ...virtualModels, [m]: { type: 'metric', strategy: 'fastest', targets: [] } });
             }}
           ><Plus size={12} /> New Alias</Button>
        </div>
        <div className="space-y-3">
          {Object.entries(virtualModels || {}).map(([name, vConfig]) => (
            <div key={name} className="rounded-xl border border-border/50 bg-muted/20 p-4 space-y-4">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <Badge variant="outline" className="font-mono text-[10px] font-black px-2 bg-background">{name}</Badge>
                  <Select value={vConfig.type} onValueChange={(t) => onUpdate({ ...virtualModels, [name]: { ...vConfig, type: t } })}>
                    <SelectTrigger className="h-7 w-28 text-[9px] font-black uppercase">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="metric">Metric</SelectItem>
                      <SelectItem value="arena">Arena</SelectItem>
                      <SelectItem value="pipeline">Pipeline</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <Button 
                  variant="ghost" size="icon" className="h-7 w-7 text-muted-foreground hover:text-destructive"
                  onClick={() => {
                    const next = { ...virtualModels };
                    delete next[name];
                    onUpdate(next);
                  }}
                ><Trash2 size={12} /></Button>
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                {vConfig.type === 'metric' && (
                  <div className="space-y-1.5">
                    <Label className="text-[9px] font-black uppercase opacity-50">Selection Strategy</Label>
                    <Select value={vConfig.strategy} onValueChange={(s) => onUpdate({ ...virtualModels, [name]: { ...vConfig, strategy: s } })}>
                      <SelectTrigger className="h-8 text-xs font-bold">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="fastest">Fastest (TTFT)</SelectItem>
                        <SelectItem value="cheapest">Cheapest (Factor)</SelectItem>
                        <SelectItem value="most_reliable">Most Reliable</SelectItem>
                        <SelectItem value="random">Random (Canary)</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                )}

                {vConfig.type === 'pipeline' && (
                  <div className="space-y-1.5">
                    <Label className="text-[9px] font-black uppercase opacity-50">Judge Model</Label>
                    <Select value={vConfig.judge_model} onValueChange={(j) => onUpdate({ ...virtualModels, [name]: { ...vConfig, judge_model: j } })}>
                      <SelectTrigger className="h-8 text-xs font-bold">
                        <SelectValue placeholder="Select Judge..." />
                      </SelectTrigger>
                      <SelectContent>
                        {availableModels.map(m => <SelectItem key={m} value={m}>{m}</SelectItem>)}
                      </SelectContent>
                    </Select>
                  </div>
                )}

                <div className="space-y-1.5 md:col-span-2">
                  <Label className="text-[9px] font-black uppercase opacity-50">Backing Target Models</Label>
                  <div className="flex flex-wrap gap-2 p-2 rounded-lg bg-background/50 border border-border/30">
                    {vConfig.targets?.map((t: string) => (
                      <Badge key={t} className="gap-1 pl-2 pr-1 h-6 text-[10px] font-bold">
                        {t}
                        <button onClick={() => {
                           const nextTargets = vConfig.targets.filter((target: string) => target !== t);
                           onUpdate({ ...virtualModels, [name]: { ...vConfig, targets: nextTargets } });
                        }} className="hover:text-red-400 p-0.5"><Trash2 size={10} /></button>
                      </Badge>
                    ))}
                    <Select onValueChange={(v) => {
                       if (vConfig.targets?.includes(v)) return;
                       const nextTargets = [...(vConfig.targets || []), v];
                       onUpdate({ ...virtualModels, [name]: { ...vConfig, targets: nextTargets } });
                    }}>
                      <SelectTrigger className="h-6 w-32 text-[9px] font-black uppercase border-dashed">
                        + Add Target
                      </SelectTrigger>
                      <SelectContent>
                        {availableModels.map(m => <SelectItem key={m} value={m}>{m}</SelectItem>)}
                      </SelectContent>
                    </Select>
                  </div>
                </div>
              </div>
            </div>
          ))}
          {Object.keys(virtualModels || {}).length === 0 && (
            <p className="py-8 text-center text-xs italic text-muted-foreground/50 border border-dashed rounded-xl">No virtual models defined</p>
          )}
        </div>
      </div>
    );
}

export const ConfigPage: React.FC = () => {
  const { status } = useCluster();
  const [config, setConfig] = useState<Config | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const [saving, setSaving] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [activeTab, setActiveTab] = useState('conn');

  const sections = [
    { value: 'conn', label: 'Connection', icon: Shield, color: 'text-emerald-400', requiresConfig: false },
    { value: 'econ', label: 'Economics', icon: BarChart2, color: 'text-amber-400', requiresConfig: true },
    { value: 'hedging', label: 'Hedging', icon: Zap, color: 'text-primary', requiresConfig: true },
    { value: 'routing', label: 'Routing', icon: BarChart2, color: 'text-purple-400', requiresConfig: true },
    { value: 'circuit', label: 'Circuit Breaker', icon: Shield, color: 'text-red-400', requiresConfig: true },
    { value: 'limits', label: 'System Limits', icon: Clock, color: 'text-teal-400', requiresConfig: true },
    { value: 'autoscaling', label: 'Auto-Scaling', icon: TrendingUp, color: 'text-pink-400', requiresConfig: true },
    { value: 'agentcaps', label: 'Agent Control', icon: Cpu, color: 'text-orange-400', requiresConfig: true },
    { value: 'virtual', label: 'Virtual Models', icon: Layers, color: 'text-sky-400', requiresConfig: true },
  ] as const;

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
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : (err as { message?: string })?.message || 'Failed to save configuration';
      toast.error(msg);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="mx-auto max-w-6xl space-y-6 p-4 pb-28 sm:p-6 sm:pb-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h2 className="text-lg font-black uppercase tracking-widest sm:text-xl">Cluster Configuration</h2>
          <p className="text-sm text-muted-foreground mt-0.5">Live runtime parameters — changes apply immediately</p>
        </div>
        <div className="hidden flex-wrap items-center gap-2 sm:flex">
          {dirty && <Badge className="text-xs font-black bg-amber-500/15 text-amber-400 border-amber-500/30">Unsaved changes</Badge>}
          <Button variant="outline" size="sm" className="h-9 text-xs font-bold gap-1.5" onClick={load} disabled={loading}>
            <RefreshCw size={14} className={loading ? 'animate-spin' : ''} /> Reload
          </Button>
          <Button size="sm" className="h-9 text-xs font-black uppercase tracking-widest gap-1.5 shadow-lg shadow-primary/20" onClick={save} disabled={saving || !dirty || error}>
            {saving ? <RefreshCw size={14} className="animate-spin" /> : <Save size={14} />}
            Apply
          </Button>
        </div>
      </div>

      <Tabs value={activeTab} onValueChange={setActiveTab} className="grid gap-4 lg:grid-cols-[240px_minmax(0,1fr)] lg:gap-6">
        <div className="space-y-3">
          <div className="sm:hidden">
            <Select value={activeTab} onValueChange={setActiveTab}>
              <SelectTrigger className="h-10 text-sm">
                <SelectValue placeholder="Select section" />
              </SelectTrigger>
              <SelectContent>
                {sections.map(section => (
                  <SelectItem key={section.value} value={section.value} disabled={section.requiresConfig && !config}>
                    {section.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <TabsList className="hidden h-auto grid-cols-1 gap-2 bg-transparent p-0 sm:grid lg:content-start">
            {sections.map(section => {
              const Icon = section.icon;
              return (
                <TabsTrigger
                  key={section.value}
                  value={section.value}
                  disabled={section.requiresConfig && !config}
                  className="h-10 justify-start gap-2 px-3 text-sm font-semibold data-[state=active]:bg-card data-[state=active]:shadow-sm lg:w-full"
                >
                  <Icon size={14} className={section.color} /> {section.label}
                </TabsTrigger>
              );
            })}
          </TabsList>
        </div>

        <div className="flex-1 min-w-0">
          <TabsContent value="conn" className="mt-0">
            <Card className="border-border/50">
              <CardHeader>
                <CardTitle className="text-sm font-black uppercase tracking-widest flex items-center gap-2">
                  <Shield size={16} className="text-emerald-400" /> Connection & Auth
                </CardTitle>
                <CardDescription className="text-xs">
                  Frontend connection parameters. These are stored locally in your browser. Leave empty to use defaults from environment.
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <FieldRow label="Balancer URL" description="Base URL of the FlakyOllama Balancer (e.g. http://localhost:8080)">
                  <Input
                    value={localUrl}
                    onChange={e => { setLocalUrl(e.target.value); setConnDirty(true); }}
                    placeholder="Default (Relative)"
                    className="h-9 bg-muted/50 border-border/50 text-sm font-mono"
                  />
                </FieldRow>
                <FieldRow label="API Token" description="Bearer token for cluster authentication">
                  <Input
                    type="password"
                    value={localToken}
                    onChange={e => { setLocalToken(e.target.value); setConnDirty(true); }}
                    placeholder="Default from ENV"
                    className="h-9 bg-muted/50 border-border/50 text-sm font-mono"
                  />
                </FieldRow>
                <div className="flex justify-end pt-4">
                  <Button
                    size="sm"
                    variant="secondary"
                    className="h-9 text-xs font-black uppercase"
                    disabled={!connDirty}
                    onClick={saveConnection}
                  >
                    Save Connection Settings
                  </Button>
                </div>
              </CardContent>
            </Card>
          </TabsContent>

          {loading ? (
            <div className="space-y-4 mt-6">
              {Array.from({ length: 4 }).map((_, i) => <Skeleton key={i} className="h-16 w-full rounded-xl" />)}
            </div>
          ) : error ? (
            <div className="p-12 text-center bg-card border border-dashed border-border rounded-xl mt-6">
              <p className="text-sm font-bold text-muted-foreground uppercase tracking-widest">Failed to connect to cluster</p>
              <p className="text-xs text-muted-foreground/60 mt-2">Check your connection settings and retry</p>
              <Button variant="outline" size="sm" className="mt-4 h-9 text-xs font-bold" onClick={load}>
                Retry Connection
              </Button>
            </div>
          ) : config && (
            <>
              <TabsContent value="econ" className="mt-0">
                <Card className="border-border/50">
                  <CardHeader>
                    <CardTitle className="text-sm font-black uppercase tracking-widest flex items-center gap-2">
                      <BarChart2 size={16} className="text-amber-400" /> Cluster Economics
                    </CardTitle>
                    <CardDescription className="text-xs">
                      Configure how agents earn credits (Reward) and how clients are charged (Cost).
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
                      <div className="space-y-4">
                        <p className="text-xs font-black uppercase tracking-widest text-amber-400/80">Agent Rewards</p>
                        <FieldRow label="Global Multiplier" description="Base reward for all nodes">
                          <NumberField
                            value={config.global_reward_multiplier}
                            onChange={v => set('global_reward_multiplier', v)}
                            step={0.1} min={0}
                          />
                        </FieldRow>
                        <Separator className="bg-border/30" />
                        <FactorTable
                          title="Model Reward Factors"
                          factors={config.model_reward_factors || {}}
                          onUpdate={(next) => set('model_reward_factors', next)}
                        />
                      </div>
                      <div className="space-y-4 lg:border-l border-border/30 lg:pl-8">
                        <p className="text-xs font-black uppercase tracking-widest text-blue-400/80">Client Costs</p>
                        <FieldRow label="Global Multiplier" description="Base charge for all clients">
                          <NumberField
                            value={config.global_cost_multiplier}
                            onChange={v => set('global_cost_multiplier', v)}
                            step={0.1} min={0}
                          />
                        </FieldRow>
                        <Separator className="bg-border/30" />
                        <FactorTable
                          title="Model Cost Factors"
                          factors={config.model_cost_factors || {}}
                          onUpdate={(next) => set('model_cost_factors', next)}
                        />
                      </div>
                    </div>
                  </CardContent>
                </Card>
              </TabsContent>

              <TabsContent value="hedging" className="mt-0">
                <Card className="border-border/50">
                  <CardHeader>
                    <CardTitle className="text-sm font-black uppercase tracking-widest flex items-center gap-2">
                      <Zap size={16} className="text-primary" /> Request Hedging
                    </CardTitle>
                    <CardDescription className="text-xs">
                      Duplicate delayed requests on an alternate node to reduce tail latency.
                    </CardDescription>
                  </CardHeader>
                  <CardContent className="space-y-4">
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
                        <div className="flex justify-between text-xs font-bold text-muted-foreground w-full max-w-xs mx-auto">
                          <span>0.0</span>
                          <span className="text-foreground font-black">{config.hedging_percentile.toFixed(2)}</span>
                          <span>1.0</span>
                        </div>
                        <Slider
                          disabled={!config.enable_hedging}
                          value={[config.hedging_percentile]}
                          onValueChange={([v]) => set('hedging_percentile', v)}
                          min={0} max={1} step={0.01}
                          className="w-full max-w-xs mx-auto"
                        />
                      </div>
                    </FieldRow>
                  </CardContent>
                </Card>
              </TabsContent>

              <TabsContent value="routing" className="mt-0">
                <Card className="border-border/50">
                  <CardHeader>
                    <CardTitle className="text-sm font-black uppercase tracking-widest flex items-center gap-2">
                      <BarChart2 size={16} className="text-purple-400" /> Routing Heuristics
                    </CardTitle>
                    <CardDescription className="text-xs">
                      Scoring weights used by the routing algorithm. Higher values increase influence.
                    </CardDescription>
                  </CardHeader>
                  <CardContent className="space-y-2">
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
                          <NumberField value={val as number} onChange={v => set(key, v)} step={0.1} min={0} />
                        </FieldRow>
                      );
                    })}
                  </CardContent>
                </Card>
              </TabsContent>

              <TabsContent value="circuit" className="mt-0">
                <Card className="border-border/50">
                  <CardHeader>
                    <CardTitle className="text-sm font-black uppercase tracking-widest flex items-center gap-2">
                      <Shield size={16} className="text-red-400" /> Circuit Breaker
                    </CardTitle>
                    <CardDescription className="text-xs">
                      Isolates unhealthy nodes after repeated failures.
                    </CardDescription>
                  </CardHeader>
                  <CardContent className="space-y-2">
                    <FieldRow label="Error Threshold" description="Consecutive errors before a node is put in cooloff">
                      <NumberField value={config.circuit_breaker.error_threshold} onChange={v => set('circuit_breaker.error_threshold', v)} min={1} />
                    </FieldRow>
                    <FieldRow label="Cooloff Duration" description="Seconds a node stays in cooloff before retrying">
                      <NumberField value={config.circuit_breaker.cooloff_sec} onChange={v => set('circuit_breaker.cooloff_sec', v)} min={1} />
                    </FieldRow>
                  </CardContent>
                </Card>
              </TabsContent>

              <TabsContent value="limits" className="mt-0">
                <Card className="border-border/50">
                  <CardHeader>
                    <CardTitle className="text-sm font-black uppercase tracking-widest flex items-center gap-2">
                      <Clock size={16} className="text-teal-400" /> System Limits & Timers
                    </CardTitle>
                    <CardDescription className="text-xs">
                      Queue depths, timeouts, and polling intervals.
                    </CardDescription>
                  </CardHeader>
                  <CardContent className="space-y-2">
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
                    <Separator className="my-4 bg-border/30" />
                    <FieldRow label="Enable Model Approval" description="Require manual approval for pulling or deleting models">
                      <div className="flex justify-end">
                        <Switch
                          checked={config.enable_model_approval}
                          onCheckedChange={v => set('enable_model_approval', v)}
                        />
                      </div>
                    </FieldRow>
                  </CardContent>
                </Card>
              </TabsContent>

              <TabsContent value="autoscaling" className="mt-0">
                <Card className="border-border/50">
                  <CardHeader>
                    <CardTitle className="text-sm font-black uppercase tracking-widest flex items-center gap-2">
                      <TrendingUp size={16} className="text-pink-400" /> Auto-Scaling Policy
                    </CardTitle>
                    <CardDescription className="text-xs">
                      Automatically deploy models to healthy nodes when queue pressure increases.
                    </CardDescription>
                  </CardHeader>
                  <CardContent className="space-y-4">
                    <FieldRow label="Enable Auto-Scaling" description="Automatically provision models based on demand">
                      <div className="flex justify-end">
                        <Switch
                          checked={config.enable_auto_scaling}
                          onCheckedChange={v => set('enable_auto_scaling', v)}
                        />
                      </div>
                    </FieldRow>
                    <FieldRow label="Scaling Threshold" description="Queue depth per model that triggers an auto-pull">
                      <NumberField
                        value={config.auto_scale_threshold}
                        onChange={v => set('auto_scale_threshold', v)}
                        min={1}
                      />
                    </FieldRow>
                  </CardContent>
                </Card>
              </TabsContent>

              <TabsContent value="agentcaps" className="mt-0">
                <Card className="border-border/50">
                  <CardHeader>
                    <CardTitle className="text-sm font-black uppercase tracking-widest flex items-center gap-2">
                      <Cpu size={16} className="text-orange-400" /> Agent Resource Control
                    </CardTitle>
                    <CardDescription className="text-xs">
                      Limits applied to all agents. 0 means unlimited.
                    </CardDescription>
                  </CardHeader>
                  <CardContent className="space-y-4">
                    <FieldRow label="Max VRAM Allocated (GB)" description="Cap total VRAM reported/used per agent">
                      <NumberField
                        value={config.max_vram_allocated / 1e9}
                        onChange={v => set('max_vram_allocated', v * 1e9)}
                        step={1} min={0}
                      />
                    </FieldRow>
                    <FieldRow label="Max CPU Cores" description="Cap total CPU cores reported/used per agent">
                      <NumberField
                        value={config.max_cpu_allocated}
                        onChange={v => set('max_cpu_allocated', v)}
                        min={0}
                      />
                    </FieldRow>
                  </CardContent>
                </Card>
              </TabsContent>

              <TabsContent value="virtual" className="mt-0">
                <Card className="border-border/50">
                  <CardHeader>
                    <CardTitle className="text-sm font-black uppercase tracking-widest flex items-center gap-2">
                      <Layers size={16} className="text-sky-400" /> Virtual Models & Pipelines
                    </CardTitle>
                    <CardDescription className="text-xs">
                      Define model aliases that intelligently route to specialists or execute multi-stage workflows.
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    <VirtualModelTable 
                      virtualModels={config.virtual_models || {}} 
                      onUpdate={(next) => set('virtual_models', next)}
                      availableModels={status?.all_models || []}
                    />
                  </CardContent>
                </Card>
              </TabsContent>
            </>
          )}
        </div>
      </Tabs>

      <Card className="fixed inset-x-3 bottom-3 z-30 border-border/60 bg-background/95 backdrop-blur supports-backdrop-filter:bg-background/70 sm:hidden">
        <CardContent className="flex items-center justify-between gap-2 p-3">
          <div className="min-w-0">
            <p className="truncate text-xs font-semibold">{dirty ? 'Unsaved changes' : 'All changes saved'}</p>
            <p className="text-[11px] text-muted-foreground">Current section: {sections.find(s => s.value === activeTab)?.label}</p>
          </div>
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" className="h-9" onClick={load} disabled={loading}>
              <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
            </Button>
            <Button size="sm" className="h-9" onClick={save} disabled={saving || !dirty || error}>
              {saving ? <RefreshCw size={14} className="animate-spin" /> : <Save size={14} />}
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
};
