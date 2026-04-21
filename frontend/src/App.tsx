import { NavLink, Outlet } from 'react-router';
import { useCluster } from './ClusterContext';
import {
  LayoutDashboard, Server, Database, Terminal, ScrollText,
  Settings, RefreshCw, Zap, ChevronRight, AlertCircle, MessageSquare,
} from 'lucide-react';
import { Toaster } from 'sonner';
import { AnimatePresence, motion } from 'framer-motion';
import { useLocation } from 'react-router';

import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Separator } from '@/components/ui/separator';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import type { ClusterStatus } from './api';

const navItems = [
  { to: '/', label: 'Overview', icon: LayoutDashboard, end: true },
  { to: '/fleet', label: 'Fleet', icon: Server },
  { to: '/registry', label: 'Registry', icon: Database },
  { to: '/playground', label: 'Playground', icon: Terminal },
  { to: '/chat', label: 'Chat', icon: MessageSquare },
  { to: '/logs', label: 'Logs', icon: ScrollText },
  { to: '/config', label: 'Configuration', icon: Settings },
];

function formatUptime(s: number) {
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  return `${h}h ${m}m`;
}

function getOfflineCount(status: ClusterStatus) {
  return Object.values(status.nodes).filter((n: any) => n.state === 2).length;
}

const App = () => {
  const { status, error, isLoading, refresh } = useCluster();
  const location = useLocation();

  const pageName = navItems.find(n =>
    n.end ? location.pathname === n.to : location.pathname.startsWith(n.to)
  )?.label ?? 'Dashboard';

  if (!status) {
    return (
      <div className="h-screen flex flex-col items-center justify-center gap-5 bg-background">
        <Toaster position="top-right" theme="dark" richColors />
        <div className="flex items-center gap-3">
          <div className="w-10 h-10 rounded-xl bg-primary/20 flex items-center justify-center">
            <Zap size={20} className="text-primary animate-pulse" />
          </div>
          <div>
            <p className="text-sm font-black uppercase tracking-widest">FlakyOllama</p>
            <p className="text-[10px] text-muted-foreground font-bold uppercase tracking-widest">Orchestrator Console</p>
          </div>
        </div>
        {error ? (
          <div className="flex flex-col items-center gap-3">
            <div className="flex items-center gap-2 text-destructive text-xs font-bold">
              <AlertCircle size={14} /> {error}
            </div>
            <Button variant="outline" size="sm" className="text-xs font-black uppercase" onClick={refresh}>
              Retry Connection
            </Button>
          </div>
        ) : (
          <div className="flex items-center gap-2 text-muted-foreground text-[10px] font-black uppercase tracking-widest">
            <RefreshCw size={12} className="animate-spin" /> Syncing compute fabric...
          </div>
        )}
      </div>
    );
  }

  const nodes = Object.values(status.nodes) as any[];
  const healthyCount = nodes.filter(n => n.state === 0 && !n.draining).length;
  const degradedCount = nodes.filter(n => n.state === 1).length;
  const offlineCount = nodes.filter(n => n.state === 2).length;
  const clusterHealthColor = offlineCount > 0 ? 'text-red-400' : degradedCount > 0 ? 'text-amber-400' : 'text-emerald-400';
  const clusterHealthLabel = offlineCount > 0 ? 'Degraded' : degradedCount > 0 ? 'Warning' : 'Healthy';

  return (
    <TooltipProvider>
      <div className="flex h-screen bg-background text-foreground overflow-hidden">
        <Toaster position="top-right" theme="dark" richColors closeButton />

        {/* Sidebar */}
        <aside className="w-56 shrink-0 flex flex-col bg-sidebar border-r border-sidebar-border h-full">
          {/* Logo */}
          <div className="px-4 py-4 border-b border-sidebar-border">
            <div className="flex items-center gap-3">
              <div className="w-8 h-8 rounded-lg bg-primary/20 flex items-center justify-center shrink-0">
                <Zap size={16} className="text-primary" fill="currentColor" fillOpacity={0.3} />
              </div>
              <div className="min-w-0">
                <p className="text-xs font-black uppercase tracking-tight leading-none">FlakyOllama</p>
                <p className="text-[9px] text-muted-foreground/60 font-bold uppercase tracking-widest mt-0.5">Console v1</p>
              </div>
            </div>
          </div>

          {/* Cluster vitals */}
          <div className="px-4 py-3 border-b border-sidebar-border">
            <div className="flex items-center justify-between mb-2">
              <span className="text-[9px] font-black uppercase tracking-widest text-muted-foreground">Cluster</span>
              <span className={`text-[9px] font-black uppercase ${clusterHealthColor}`}>{clusterHealthLabel}</span>
            </div>
            <div className="grid grid-cols-3 gap-1 text-center">
              <div className="bg-emerald-500/10 rounded-md py-1.5">
                <p className="text-xs font-black text-emerald-400">{healthyCount}</p>
                <p className="text-[8px] text-emerald-500/60 font-bold">OK</p>
              </div>
              <div className={`${degradedCount > 0 ? 'bg-amber-500/10' : 'bg-muted/30'} rounded-md py-1.5`}>
                <p className={`text-xs font-black ${degradedCount > 0 ? 'text-amber-400' : 'text-muted-foreground/30'}`}>{degradedCount}</p>
                <p className="text-[8px] text-muted-foreground/40 font-bold">WARN</p>
              </div>
              <div className={`${offlineCount > 0 ? 'bg-red-500/10' : 'bg-muted/30'} rounded-md py-1.5`}>
                <p className={`text-xs font-black ${offlineCount > 0 ? 'text-red-400' : 'text-muted-foreground/30'}`}>{offlineCount}</p>
                <p className="text-[8px] text-muted-foreground/40 font-bold">DOWN</p>
              </div>
            </div>
          </div>

          {/* Navigation */}
          <nav className="flex-1 px-2 py-3 space-y-0.5 overflow-y-auto">
            {navItems.map(item => {
              const offlineN = item.to === '/fleet' ? getOfflineCount(status) : 0;
              return (
                <NavLink
                  key={item.to}
                  to={item.to}
                  end={item.end}
                  className={({ isActive }) => `
                    w-full flex items-center gap-3 px-3 py-2 rounded-lg text-left transition-all duration-150 group
                    ${isActive
                      ? 'bg-sidebar-accent text-sidebar-accent-foreground font-black'
                      : 'text-muted-foreground hover:bg-sidebar-accent/50 hover:text-sidebar-accent-foreground font-bold'
                    }
                  `}
                >
                  {({ isActive }) => (
                    <>
                      <item.icon size={15} className={isActive ? 'text-primary' : 'text-muted-foreground group-hover:text-foreground'} />
                      <span className="text-xs uppercase tracking-widest flex-1">{item.label}</span>
                      {offlineN > 0 && (
                        <Badge className="text-[8px] font-black h-4 px-1.5 bg-destructive/20 text-destructive border-destructive/30 min-w-[1rem] justify-center">
                          {offlineN}
                        </Badge>
                      )}
                      {isActive && <ChevronRight size={12} className="text-primary shrink-0" />}
                    </>
                  )}
                </NavLink>
              );
            })}
          </nav>

          {/* Stats footer */}
          <div className="px-4 py-3 border-t border-sidebar-border space-y-1.5">
            <div className="flex justify-between text-[9px] font-bold text-muted-foreground">
              <span>Uptime</span><span className="text-foreground">{formatUptime(status.uptime_seconds)}</span>
            </div>
            <div className="flex justify-between text-[9px] font-bold text-muted-foreground">
              <span>Queue</span>
              <span className={status.queue_depth > 0 ? 'text-amber-400 font-black' : 'text-foreground'}>{status.queue_depth}</span>
            </div>
            <div className="flex justify-between text-[9px] font-bold text-muted-foreground">
              <span>Active</span><span className="text-foreground">{status.active_workloads}</span>
            </div>
          </div>
        </aside>

        {/* Main content */}
        <div className="flex flex-col flex-1 min-w-0 overflow-hidden">
          {/* Top bar */}
          <header className="h-[57px] border-b border-border/50 bg-background/80 backdrop-blur-sm flex items-center justify-between px-6 shrink-0 z-10">
            <div className="flex items-center gap-3">
              <h1 className="text-sm font-black uppercase tracking-widest">{pageName}</h1>
              {error && (
                <Badge className="text-[9px] font-black bg-destructive/15 text-destructive border-destructive/30 gap-1">
                  <AlertCircle size={9} /> {error}
                </Badge>
              )}
            </div>
            <div className="flex items-center gap-3">
              <div className="flex items-center gap-4 text-[9px] font-black uppercase text-muted-foreground">
                <span className="flex items-center gap-1.5">
                  <span className="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse" />
                  {status.active_workloads} active
                </span>
                <Separator orientation="vertical" className="h-4" />
                <span>Avg CPU {status.avg_cpu_usage.toFixed(0)}%</span>
              </div>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost" size="icon"
                    className="h-8 w-8 text-muted-foreground"
                    onClick={refresh}
                    disabled={isLoading}
                  >
                    <RefreshCw size={14} className={isLoading ? 'animate-spin' : ''} />
                  </Button>
                </TooltipTrigger>
                <TooltipContent className="text-[10px] font-bold">Refresh cluster state</TooltipContent>
              </Tooltip>
            </div>
          </header>

          {/* Routed page */}
          <main className="flex-1 overflow-auto">
            <AnimatePresence mode="wait">
              <motion.div
                key={location.pathname}
                initial={{ opacity: 0, y: 8 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0, y: -8 }}
                transition={{ duration: 0.15, ease: 'easeInOut' }}
                className="h-full"
              >
                <Outlet />
              </motion.div>
            </AnimatePresence>
          </main>
        </div>
      </div>
    </TooltipProvider>
  );
};

export default App;
