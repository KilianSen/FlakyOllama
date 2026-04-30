import React from 'react';
import { NavLink } from 'react-router';
import { Zap } from 'lucide-react';
import { motion } from 'framer-motion';
import { Button } from '@/components/ui/button';
import { useCluster } from '../ClusterContext';

export const LandingPage: React.FC = () => {
  const { status } = useCluster();

  const nodes = status ? Object.values(status.nodes) as any[] : [];
  const healthyCount = nodes.filter(n => n.state === 0 && !n.draining).length;

  const stats = [
    { label: 'Active Nodes', value: status ? healthyCount : null },
    { label: 'Models Available', value: status ? status.all_models.length : null },
    { label: 'Active Workloads', value: status ? status.active_workloads : null },
  ];

  return (
    <motion.div
      initial={{ opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.25, ease: 'easeOut' }}
      className="min-h-screen bg-background flex flex-col items-center justify-center px-6 text-center"
    >
      {/* Brand mark */}
      <div className="flex items-center gap-3 mb-6">
        <div className="w-12 h-12 rounded-2xl bg-primary/20 flex items-center justify-center">
          <Zap size={26} className="text-primary" fill="currentColor" fillOpacity={0.4} />
        </div>
        <div className="text-left">
          <p className="text-2xl font-black uppercase tracking-tight leading-none">FlakyOllama</p>
          <p className="text-[11px] text-muted-foreground font-black uppercase tracking-widest mt-0.5">
            Distributed Inference Fabric
          </p>
        </div>
      </div>

      {/* Stat cards */}
      <div className="grid grid-cols-3 gap-4 mb-10 w-full max-w-lg">
        {stats.map(stat => (
          <div
            key={stat.label}
            className="rounded-xl bg-card border border-border/50 px-4 py-4 flex flex-col items-center gap-1"
          >
            <span className="text-2xl font-black text-foreground">
              {stat.value !== null ? stat.value : '—'}
            </span>
            <span className="text-[9px] font-black uppercase tracking-widest text-muted-foreground">
              {stat.label}
            </span>
          </div>
        ))}
      </div>

      {/* CTA buttons */}
      <div className="flex items-center gap-3 mb-6">
        {status?.oidc_enabled && (
          <Button
            className="h-10 px-6 font-black uppercase text-xs tracking-widest shadow-lg shadow-primary/20"
            onClick={() => { window.location.href = '/auth/login'; }}
          >
            Sign In
          </Button>
        )}
        <NavLink to="/portal">
          <Button
            variant="outline"
            className="h-10 px-6 font-black uppercase text-xs tracking-widest"
          >
            Browse Marketplace
          </Button>
        </NavLink>
      </div>

      {/* Tagline */}
      <p className="text-xs text-muted-foreground max-w-sm leading-relaxed">
        Connect your compute, access high-performance models, earn credits.
      </p>
    </motion.div>
  );
};
