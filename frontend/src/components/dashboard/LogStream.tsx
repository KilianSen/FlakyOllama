import React, { useState, useEffect } from 'react';
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import sdk from '../../api';

export const LogStream: React.FC = () => {
  const [logs, setLogs] = useState<string[]>([]);

  useEffect(() => {
    const cleanup = sdk.streamLogs((msg) => {
      setLogs(prev => [...prev.slice(-99), msg]);
    });
    return cleanup;
  }, []);

  return (
    <Card className="border-none shadow-sm bg-slate-950">
      <CardHeader className="py-3 px-6 border-b border-white/5 flex flex-row items-center justify-between">
        <div className="flex items-center gap-2">
          <div className="w-2 h-2 rounded-full bg-primary animate-pulse" />
          <CardTitle className="text-[9px] font-black uppercase tracking-[0.3em] text-slate-400">Live Telemetry Stream</CardTitle>
        </div>
        <Button variant="ghost" size="sm" onClick={() => setLogs([])} className="h-6 text-[8px] font-black uppercase text-slate-500 hover:text-white">Flush buffer</Button>
      </CardHeader>
      <CardContent className="p-0">
        <ScrollArea className="h-[200px] font-mono text-[10px] text-indigo-300/70">
          <div className="p-6 flex flex-col-reverse gap-1.5">
            {[...logs].reverse().map((log, i) => (
              <div key={i} className="flex gap-4 border-l border-primary/20 pl-4 py-0.5 hover:bg-white/5 transition-all">
                <span className="text-slate-600 shrink-0 font-black tracking-tighter">[{new Date().toLocaleTimeString()}]</span>
                <p className="tracking-tight">{log}</p>
              </div>
            ))}
          </div>
        </ScrollArea>
      </CardContent>
    </Card>
  );
};
