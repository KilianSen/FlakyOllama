import React, { useMemo } from 'react';
import { motion } from 'framer-motion';
import { Zap, Cpu, Server, Activity, Database, Clock, Layers } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import {
  BarChart, Bar, XAxis, YAxis, CartesianGrid,
  Tooltip as RechartTooltip, ResponsiveContainer
} from 'recharts';
import type { NodeStatus } from '../api';
import { useCluster } from '../ClusterContext';
import { computeRoutability, LATENCY_HINTS } from '../lib/modelUtils';

function formatBytes(bytes: number, decimals = 1) {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(decimals)) + ' ' + sizes[i];
}

function formatUptime(seconds: number) {
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  return `${h}h ${m}m`;
}

const nodeStateColor = (node: NodeStatus) => {
  if (node.state === 2) return '#ef4444';
  if (node.state === 1) return '#f59e0b';
  return '#10b981';
};

export const OverviewPage: React.FC = () => {
  const { status } = useCluster();
  if (!status) return null;
  const nodes = Object.values(status.nodes) as NodeStatus[];
  const healthyNodes = nodes.filter(n => n.state === 0).length;
  const degradedNodes = nodes.filter(n => n.state === 1).length;
  const offlineNodes = nodes.filter(n => n.state === 2).length;

  const vramData = useMemo(() => nodes.map(n => ({
    name: n.id.split('-').pop() ?? n.id,
    used: parseFloat((n.vram_used / 1e9).toFixed(1)),
    total: parseFloat((n.vram_total / 1e9).toFixed(1)),
    free: parseFloat(((n.vram_total - n.vram_used) / 1e9).toFixed(1)),
  })), [nodes]);

  const cpuData = useMemo(() => nodes.map(n => ({
    name: n.id.split('-').pop() ?? n.id,
    cpu: parseFloat(n.cpu_usage.toFixed(1)),
    mem: parseFloat(n.memory_usage.toFixed(1)),
  })), [nodes]);

  const kpis = [
    {
      title: 'Cluster Nodes',
      value: nodes.length,
      sub: `${healthyNodes} healthy · ${degradedNodes} degraded · ${offlineNodes} offline`,
      icon: Server,
      color: 'text-blue-400',
    },
    {
      title: 'Total VRAM',
      value: formatBytes(status.total_vram),
      sub: `${formatBytes(status.used_vram)} used`,
      icon: Layers,
      color: 'text-purple-400',
    },
    {
      title: 'Active Workloads',
      value: status.active_workloads,
      sub: `${status.queue_depth} queued`,
      icon: Activity,
      color: status.queue_depth > 0 ? 'text-amber-400' : 'text-emerald-400',
      pulse: status.queue_depth > 0,
    },
    {
      title: 'Avg CPU Load',
      value: `${status.avg_cpu_usage.toFixed(1)}%`,
      sub: `${status.total_cpu_cores} total cores`,
      icon: Cpu,
      color: status.avg_cpu_usage > 80 ? 'text-red-400' : 'text-sky-400',
    },
    {
      title: 'Models in Fleet',
      value: status.all_models?.length ?? 0,
      sub: 'across all nodes',
      icon: Database,
      color: 'text-indigo-400',
    },
    {
      title: 'Uptime',
      value: formatUptime(status.uptime_seconds),
      sub: 'balancer runtime',
      icon: Clock,
      color: 'text-teal-400',
    },
  ];

  // topology
  const centerX = 220;
  const centerY = 160;
  const radius = 120;

  return (
    <div className="space-y-6 p-6">
      {/* KPI Grid */}
      <div className="grid grid-cols-2 xl:grid-cols-3 gap-4">
        {kpis.map((kpi) => (
          <Card key={kpi.title} className="bg-card border-border/50">
            <CardContent className="p-5">
              <div className="flex items-start justify-between">
                <div className="space-y-1">
                  <p className="text-[10px] font-bold uppercase tracking-widest text-muted-foreground">{kpi.title}</p>
                  <p className={`text-2xl font-black tracking-tighter ${kpi.color} ${kpi.pulse ? 'animate-pulse' : ''}`}>{kpi.value}</p>
                  <p className="text-[10px] text-muted-foreground/60">{kpi.sub}</p>
                </div>
                <div className="p-2 rounded-lg bg-muted/50">
                  <kpi.icon size={16} className={kpi.color} />
                </div>
              </div>
            </CardContent>
          </Card>
        ))}
      </div>

      {/* Topology + VRAM Chart */}
      <div className="grid grid-cols-1 lg:grid-cols-5 gap-4">
        {/* Topology */}
        <Card className="lg:col-span-2 bg-card border-border/50">
          <CardHeader className="py-3 border-b border-border/50">
            <CardTitle className="text-[10px] font-black uppercase tracking-widest text-muted-foreground flex items-center gap-2">
              <Zap size={12} className="text-primary" /> Routing Topology
            </CardTitle>
          </CardHeader>
          <CardContent className="p-4">
            <div className="relative w-full overflow-hidden rounded-lg bg-muted/20 border border-dashed border-border/40" style={{ minHeight: 280 }}>
              <div className="absolute inset-0 bg-[radial-gradient(oklch(1_0_0/5%)_1px,transparent_1px)] [background-size:20px_20px]" />
              <svg viewBox="0 0 440 320" className="w-full h-auto">
                {/* Balancer center */}
                <circle cx={centerX} cy={centerY} r={26} fill="oklch(0.7 0.15 250)" opacity={0.15} />
                <circle cx={centerX} cy={centerY} r={18} fill="oklch(0.7 0.15 250)" />
                <text x={centerX} y={centerY + 4} textAnchor="middle" fill="white" fontSize={9} fontWeight="900">BAL</text>

                {nodes.map((node, i) => {
                  const angle = (i / nodes.length) * 2 * Math.PI - Math.PI / 2;
                  const x = centerX + Math.cos(angle) * radius;
                  const y = centerY + Math.sin(angle) * radius;
                  const isActive = (node.active_models?.length || 0) > 0;
                  const workload = status.node_workloads?.[node.address] || 0;
                  const color = nodeStateColor(node);

                  return (
                    <g key={node.address}>
                      <line
                        x1={centerX} y1={centerY} x2={x} y2={y}
                        stroke={isActive ? 'oklch(0.7 0.15 250)' : 'oklch(1 0 0 / 10%)'}
                        strokeWidth={1 + workload * 1.5}
                        strokeDasharray={node.draining || node.state === 2 ? '4 2' : '0'}
                      />
                      {workload > 0 && (
                        <motion.circle
                          r={2.5}
                          fill="oklch(0.7 0.15 250)"
                          animate={{ cx: [centerX, x], cy: [centerY, y] }}
                          transition={{ duration: Math.max(0.8, 2 - workload * 0.3), repeat: Infinity, ease: 'linear', delay: i * 0.3 }}
                        />
                      )}
                      <circle cx={x} cy={y} r={16} fill={node.has_gpu ? 'oklch(0.22 0.05 270)' : 'oklch(0.22 0 0)'} stroke={color} strokeWidth={2} />
                      <text x={x} y={y + 4} textAnchor="middle" fill={color} fontSize={8} fontWeight="900">
                        {node.has_gpu ? '⚡' : '●'}
                      </text>
                      <text x={x} y={y + 30} textAnchor="middle" fill="oklch(0.65 0 0)" fontSize={7} fontWeight="700">
                        {node.id.split('-').pop()}
                      </text>
                    </g>
                  );
                })}
              </svg>
              <div className="absolute bottom-3 left-3 flex gap-3 text-[9px] font-bold uppercase text-muted-foreground">
                <span className="flex items-center gap-1"><span className="w-2 h-2 rounded-full bg-emerald-400 inline-block" />Ready</span>
                <span className="flex items-center gap-1"><span className="w-2 h-2 rounded-full bg-amber-400 inline-block" />Degraded</span>
                <span className="flex items-center gap-1"><span className="w-2 h-2 rounded-full bg-red-400 inline-block" />Offline</span>
              </div>
            </div>
          </CardContent>
        </Card>

        {/* VRAM Chart */}
        <Card className="lg:col-span-3 bg-card border-border/50">
          <CardHeader className="py-3 border-b border-border/50">
            <CardTitle className="text-[10px] font-black uppercase tracking-widest text-muted-foreground flex items-center gap-2">
              <Layers size={12} className="text-purple-400" /> VRAM Utilization per Node
            </CardTitle>
          </CardHeader>
          <CardContent className="p-4">
            {vramData.some(d => d.total > 0) ? (
              <ResponsiveContainer width="100%" height={260}>
                <BarChart data={vramData} margin={{ top: 8, right: 8, left: -20, bottom: 0 }}>
                  <CartesianGrid strokeDasharray="3 3" stroke="oklch(1 0 0 / 5%)" />
                  <XAxis dataKey="name" tick={{ fill: 'oklch(0.65 0 0)', fontSize: 10, fontWeight: 700 }} axisLine={false} tickLine={false} />
                  <YAxis tick={{ fill: 'oklch(0.65 0 0)', fontSize: 9 }} unit=" GB" axisLine={false} tickLine={false} />
                  <RechartTooltip
                    contentStyle={{ background: 'oklch(0.175 0 0)', border: '1px solid oklch(1 0 0 / 8%)', borderRadius: 8, fontSize: 11 }}
                    labelStyle={{ color: 'oklch(0.985 0 0)', fontWeight: 700 }}
                    itemStyle={{ color: 'oklch(0.65 0 0)' }}
                    formatter={(val: unknown) => [`${val} GB`]}
                  />
                  <Bar dataKey="used" name="Used" fill="oklch(0.7 0.15 250)" radius={[3, 3, 0, 0]} />
                  <Bar dataKey="free" name="Free" fill="oklch(0.22 0 0)" radius={[3, 3, 0, 0]} />
                </BarChart>
              </ResponsiveContainer>
            ) : (
              <div className="h-[260px] flex items-center justify-center text-muted-foreground text-xs font-bold uppercase tracking-widest">
                No GPU nodes detected
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {/* CPU Load chart */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <Card className="bg-card border-border/50">
          <CardHeader className="py-3 border-b border-border/50">
            <CardTitle className="text-[10px] font-black uppercase tracking-widest text-muted-foreground flex items-center gap-2">
              <Cpu size={12} className="text-sky-400" /> Resource Load by Node
            </CardTitle>
          </CardHeader>
          <CardContent className="p-4">
            <ResponsiveContainer width="100%" height={180}>
              <BarChart data={cpuData} margin={{ top: 8, right: 8, left: -20, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="oklch(1 0 0 / 5%)" />
                <XAxis dataKey="name" tick={{ fill: 'oklch(0.65 0 0)', fontSize: 10, fontWeight: 700 }} axisLine={false} tickLine={false} />
                <YAxis tick={{ fill: 'oklch(0.65 0 0)', fontSize: 9 }} unit="%" domain={[0, 100]} axisLine={false} tickLine={false} />
                <RechartTooltip
                  contentStyle={{ background: 'oklch(0.175 0 0)', border: '1px solid oklch(1 0 0 / 8%)', borderRadius: 8, fontSize: 11 }}
                  labelStyle={{ color: 'oklch(0.985 0 0)', fontWeight: 700 }}
                  itemStyle={{ color: 'oklch(0.65 0 0)' }}
                  formatter={(val: unknown) => [`${val}%`]}
                />
                <Bar dataKey="cpu" name="CPU" fill="oklch(0.7 0.15 250 / 80%)" radius={[3, 3, 0, 0]} />
                <Bar dataKey="mem" name="Memory" fill="oklch(0.7 0.18 160 / 80%)" radius={[3, 3, 0, 0]} />
              </BarChart>
            </ResponsiveContainer>
          </CardContent>
        </Card>

        {/* Model Landscape */}
        <Card className="bg-card border-border/50">
          <CardHeader className="py-3 border-b border-border/50">
            <CardTitle className="text-[10px] font-black uppercase tracking-widest text-muted-foreground flex items-center gap-2">
              <Database size={12} className="text-indigo-400" /> Model Landscape
            </CardTitle>
          </CardHeader>
          <CardContent className="p-4">
            <div className="space-y-4">
              <div className="flex gap-2">
                <div className="flex-1 bg-muted/20 rounded-lg p-3 border border-border/30">
                   <p className="text-[9px] font-black text-muted-foreground uppercase">Availability</p>
                   <div className="flex items-center justify-between mt-1">
                      <span className="text-xl font-black text-emerald-400">
                        {status.all_models?.filter(m => computeRoutability(m, status).hotCount > 0).length}
                      </span>
                      <span className="text-[10px] font-bold text-muted-foreground uppercase">Hot Models</span>
                   </div>
                </div>
                <div className="flex-1 bg-muted/20 rounded-lg p-3 border border-border/30">
                   <p className="text-[9px] font-black text-muted-foreground uppercase">Sync Status</p>
                   <div className="flex items-center justify-between mt-1">
                      <span className="text-xl font-black text-primary">
                        {Object.keys(status.in_progress_pulls || {}).length}
                      </span>
                      <span className="text-[10px] font-bold text-muted-foreground uppercase">Syncing</span>
                   </div>
                </div>
              </div>

              <div className="space-y-1.5">
                <p className="text-[9px] font-black text-muted-foreground uppercase mb-2">Top Routable Models</p>
                {status.all_models?.slice(0, 4).map(m => {
                  const r = computeRoutability(m, status);
                  const hint = LATENCY_HINTS[r.latencyHint];
                  return (
                    <div key={m} className="flex items-center justify-between py-1.5 border-b border-white/5 last:border-0">
                       <span className="text-[11px] font-bold font-mono truncate max-w-[140px]">{m}</span>
                       <div className="flex items-center gap-2">
                          <span className={`text-[9px] font-black uppercase ${hint.color}`}>{hint.label}</span>
                          <span className="text-[9px] font-bold text-muted-foreground">{r.hotCount}/{r.totalNodes} nodes</span>
                       </div>
                    </div>
                  );
                })}
              </div>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
};
