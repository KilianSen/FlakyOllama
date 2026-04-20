import React from 'react';
import { Activity } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import type { ClusterStatus } from '../../api';

interface FabricHealthProps {
  status: ClusterStatus;
}

export const FabricHealth: React.FC<FabricHealthProps> = ({ status }) => {
  const formatBytes = (bytes: number) => {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
  };

  const kpis = [
    { 
      label: 'Cluster Nodes', 
      val: Object.keys(status.nodes).length, 
      sub: `${status.total_cpu_cores} Cores Total`, 
      color: 'text-blue-600' 
    },
    { 
      label: 'Compute Power', 
      val: formatBytes(status.total_vram), 
      sub: `${formatBytes(status.used_vram)} Used`, 
      color: 'text-indigo-600' 
    },
    { 
      label: 'Backlog', 
      val: status.queue_depth, 
      sub: status.queue_depth > 0 ? 'HEDGING DISABLED' : 'Pending Requests', 
      color: status.queue_depth > 0 ? 'text-destructive font-black' : 'text-amber-600',
      pulse: status.queue_depth > 0
    },
    { 
      label: 'System Load', 
      val: `${status.avg_cpu_usage.toFixed(0)}%`, 
      sub: `Avg CPU Usage`, 
      color: 'text-emerald-600' 
    },
  ];

  return (
    <Card className="border-none shadow-sm bg-background">
      <CardHeader className="py-4 border-b">
        <CardTitle className="text-xs font-black uppercase tracking-widest flex items-center gap-2">
          <Activity size={16} className="text-primary" /> Fabric Health
        </CardTitle>
      </CardHeader>
      <CardContent className="p-6 space-y-6">
        {kpis.map((kpi, i) => (
          <div key={i} className={`flex items-end justify-between border-b border-dashed pb-4 last:border-0 last:pb-0 ${kpi.pulse ? 'animate-pulse' : ''}`}>
            <div className="flex flex-col">
              <span className="text-[9px] font-black uppercase text-muted-foreground tracking-widest">{kpi.label}</span>
              <span className={`text-[10px] font-bold uppercase ${kpi.pulse ? 'text-destructive' : 'text-muted-foreground/40'}`}>{kpi.sub}</span>
            </div>
            <span className={`text-2xl font-black tracking-tighter ${kpi.color}`}>{kpi.val}</span>
          </div>
        ))}
      </CardContent>
    </Card>
  );
};
