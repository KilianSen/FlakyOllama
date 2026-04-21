import { useState, useEffect } from 'react';
import sdk from './api';
import type { ClusterStatus } from './api';
import { RefreshCw, Zap, Network, Settings, Activity } from 'lucide-react';
import { Toaster } from 'sonner';

// Custom Components
import { Topology } from './components/dashboard/Topology';
import { FabricHealth } from './components/dashboard/FabricHealth';
import { DistributedRegistry } from './components/dashboard/DistributedRegistry';
import { InfrastructureFleet } from './components/dashboard/InfrastructureFleet';
import { InferencePlayground } from './components/dashboard/InferencePlayground';
import { LogStream } from './components/dashboard/LogStream';
import { SettingsModal } from './SettingsModal';

// Shadcn UI Components
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";

const App = () => {
  const [status, setStatus] = useState<ClusterStatus | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [isSettingsOpen, setIsSettingsOpen] = useState(false);

  const fetchStatus = async () => {
    try {
      const data = await sdk.getStatus();
      setStatus(data);
      setError(null);
    } catch (err: any) {
      console.error('Connection failure:', err);
      setError(err.message);
    }
  };

  useEffect(() => {
    fetchStatus().then();
    const interval = setInterval(fetchStatus, 5000);
    return () => clearInterval(interval);
  }, []);

  if (!status) return (
    <div className="h-screen flex flex-col items-center justify-center gap-4 bg-background">
      <RefreshCw className="animate-spin text-primary" size={32} />
      <span className="font-black text-[10px] uppercase tracking-[0.4em] text-muted-foreground">
        {error ? `Fabric Offline: ${error}` : 'Syncing Compute Fabric'}
      </span>
      {error && (
        <Button variant="outline" size="sm" onClick={fetchStatus} className="mt-4 font-black uppercase text-[10px] tracking-widest">
          Retry Handshake
        </Button>
      )}
    </div>
  );

  return (
    <div className="min-h-screen bg-slate-50/30 flex flex-col font-sans selection:bg-primary/10 text-slate-900">
      <Toaster position="top-center" richColors closeButton />
      <SettingsModal isOpen={isSettingsOpen} onClose={() => setIsSettingsOpen(false)} />
      
      <header className="border-b bg-background/80 backdrop-blur-md px-8 h-14 flex items-center justify-between shrink-0 sticky top-0 z-50">
        <div className="flex items-center gap-4">
          <div className="w-8 h-8 rounded-lg bg-primary flex items-center justify-center">
            <Zap className="text-primary-foreground" size={18} fill="currentColor" fillOpacity={0.2} />
          </div>
          <h1 className="text-sm font-black tracking-tighter leading-none uppercase">
            FlakyOllama <span className="text-primary opacity-50">/</span> Orchestrator v1
          </h1>
        </div>
        
        <div className="flex items-center gap-6 text-[10px] font-bold text-muted-foreground uppercase tracking-widest">
          <div className="flex items-center gap-2">
            <div className="w-1.5 h-1.5 rounded-full bg-emerald-500 animate-pulse" />
            {status.active_workloads} ACTIVE TASKS
          </div>
          <div className="flex items-center gap-2">
            <div className="w-1.5 h-1.5 rounded-full bg-blue-500" />
            {Math.floor(status.uptime_seconds / 3600)}H UPTIME
          </div>
          <Separator orientation="vertical" className="h-6" />
          <div className="flex items-center gap-2">
            <Button variant="ghost" size="icon" className="h-8 w-8" onClick={() => setIsSettingsOpen(true)}>
              <Settings size={14} />
            </Button>
            <Button variant="ghost" size="icon" className="h-8 w-8" onClick={fetchStatus}>
              <RefreshCw size={14} />
            </Button>
          </div>
        </div>
      </header>

      <main className="flex-1 max-w-[1600px] w-full mx-auto p-8 space-y-8">
        
        {/* Top Section: Topology and KPIs */}
        <div className="grid grid-cols-1 lg:grid-cols-4 gap-8">
          <Card className="lg:col-span-3 border-none shadow-sm bg-background">
            <CardHeader className="py-4 border-b">
              <CardTitle className="text-xs font-black uppercase tracking-widest flex items-center gap-2">
                <Network size={16} className="text-primary" /> Routing Topology
              </CardTitle>
            </CardHeader>
            <CardContent className="p-6">
              <Topology status={status} />
            </CardContent>
          </Card>

          <FabricHealth status={status} />
        </div>

        {/* Middle Section: Registry and Infrastructure */}
        <DistributedRegistry status={status} onRefresh={fetchStatus} />
        
        <InfrastructureFleet status={status} onRefresh={fetchStatus} />

        {/* Bottom Section: Playground and Details */}
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
          <div className="lg:col-span-2">
            <InferencePlayground status={status} />
          </div>

          <div className="space-y-8 flex flex-col">
            <Card className="border-none shadow-sm bg-slate-900 text-white flex-1 overflow-hidden">
              <CardHeader className="py-4 border-b border-white/5 bg-white/5 flex flex-row items-center justify-between">
                <CardTitle className="text-[10px] font-black uppercase tracking-widest text-primary flex items-center gap-2">
                  <Settings size={14} className="text-primary" /> Orchestration Details
                </CardTitle>
                <div className="flex items-center gap-2 text-[8px] font-black text-slate-500 uppercase tracking-tighter">
                  <div className="w-1.5 h-1.5 rounded-full bg-blue-500 animate-pulse" />
                  SDK Active
                </div>
              </CardHeader>
              <CardContent className="p-6 space-y-6">
                <div className="space-y-4">
                  <div className="flex items-start gap-4">
                    <div className="w-8 h-8 rounded-xl bg-white/10 flex items-center justify-center flex-shrink-0">
                      <Zap className="w-4 h-4 text-primary" />
                    </div>
                    <div className="space-y-1">
                      <p className="text-[10px] font-black uppercase tracking-tight leading-none text-slate-200">Actor-Based State</p>
                      <p className="text-[9px] font-medium text-white/40 leading-relaxed italic">The backend uses a single-threaded actor model for 100% thread-safe cluster state management.</p>
                    </div>
                  </div>
                  <div className="flex items-start gap-4">
                    <div className="w-8 h-8 rounded-xl bg-white/10 flex items-center justify-center flex-shrink-0">
                      <Activity className="w-4 h-4 text-emerald-400" />
                    </div>
                    <div className="space-y-1">
                      <p className="text-[10px] font-black uppercase tracking-tight leading-none text-slate-200">Async Job Queue</p>
                      <p className="text-[9px] font-medium text-white/40 leading-relaxed italic">Long-running operations like model pulls are executed asynchronously with progress tracking via the SDK.</p>
                    </div>
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>
        </div>

        {/* Logs */}
        <LogStream />
      </main>
    </div>
  );
};

export default App;
