import React, { useState, useEffect, useRef } from 'react';
import { Pause, Play, Trash2, Search } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import sdk from '../api';

interface LogEntry {
  timestamp: Date;
  raw: string;
  level: 'info' | 'warn' | 'error' | 'debug';
}

function detectLevel(msg: string): LogEntry['level'] {
  const lower = msg.toLowerCase();
  if (lower.includes('error') || lower.includes('fail') || lower.includes('fatal')) return 'error';
  if (lower.includes('warn') || lower.includes('warning')) return 'warn';
  if (lower.includes('debug') || lower.includes('trace')) return 'debug';
  return 'info';
}

const levelColor: Record<LogEntry['level'], string> = {
  info:  'text-blue-400',
  warn:  'text-amber-400',
  error: 'text-red-400',
  debug: 'text-muted-foreground',
};

const levelBadge: Record<LogEntry['level'], string> = {
  info:  'bg-blue-500/10 text-blue-400 border-blue-500/20',
  warn:  'bg-amber-500/10 text-amber-400 border-amber-500/20',
  error: 'bg-red-500/10 text-red-400 border-red-500/20',
  debug: 'bg-muted text-muted-foreground border-border',
};

export const LogsPage: React.FC = () => {
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [paused, setPaused] = useState(false);
  const [filter, setFilter] = useState('');
  const [levelFilter, setLevelFilter] = useState<LogEntry['level'] | 'all'>('all');
  const pausedRef = useRef(paused);
  const bottomRef = useRef<HTMLDivElement>(null);

  pausedRef.current = paused;

  useEffect(() => {
    let active = true;
    let reader: ReadableStreamDefaultReader | null = null;

    const startStream = async () => {
      try {
        const stream = await sdk.getLogs();
        reader = stream.getReader();
        const decoder = new TextDecoder();
        let buffer = '';

        while (active) {
          const { done, value } = await reader.read();
          if (done) break;

          buffer += decoder.decode(value, { stream: true });
          const lines = buffer.split('\n\n');
          buffer = lines.pop() || '';

          for (const line of lines) {
            if (!line.startsWith('data: ')) continue;
            const msg = line.substring(6);
            if (pausedRef.current) continue;
            
            let displayMsg = msg;
            let level: LogEntry['level'] = detectLevel(msg);
            let timestamp = new Date();

            try {
              const parsed = JSON.parse(msg);
              if (parsed.message) {
                displayMsg = `[${parsed.node_id || 'unknown'}] ${parsed.message}`;
                if (parsed.level) level = (parsed.level.toLowerCase() as LogEntry['level']) || level;
                if (parsed.timestamp) timestamp = new Date(parsed.timestamp);
              }
            } catch (e) {
              // Not JSON, use raw
            }

            setLogs(prev => [
              ...prev.slice(-299),
              { timestamp, raw: displayMsg, level }
            ]);
          }
        }
      } catch (err) {
        console.error('Log stream error:', err);
      }
    };

    startStream();

    return () => {
      active = false;
      reader?.cancel();
    };
  }, []);

  useEffect(() => {
    if (!paused) bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [logs, paused]);

  const filtered = logs.filter(l => {
    if (levelFilter !== 'all' && l.level !== levelFilter) return false;
    if (filter && !l.raw.toLowerCase().includes(filter.toLowerCase())) return false;
    return true;
  });

  return (
    <div className="flex flex-col h-[calc(100vh-57px)]">
      {/* Controls */}
      <div className="flex items-center gap-3 px-6 py-3 border-b border-border/50 shrink-0">
        <div className="flex items-center gap-1.5">
          <div className={`w-2 h-2 rounded-full ${paused ? 'bg-muted-foreground' : 'bg-emerald-400 animate-pulse'}`} />
          <span className="text-[9px] font-black uppercase tracking-widest text-muted-foreground">
            {paused ? 'Paused' : 'Live'}
          </span>
        </div>
        <Badge variant="outline" className="text-[9px] font-black h-5">{filtered.length} entries</Badge>

        <div className="relative ml-auto w-64">
          <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 text-muted-foreground" size={11} />
          <Input
            placeholder="Filter logs..."
            value={filter}
            onChange={e => setFilter(e.target.value)}
            className="pl-8 h-7 text-xs bg-muted/50 border-border/50 font-mono"
          />
        </div>

        {/* Level filters */}
        <div className="flex items-center gap-1">
          {(['all', 'info', 'warn', 'error', 'debug'] as const).map(l => (
            <button
              key={l}
              onClick={() => setLevelFilter(l)}
              className={`text-[9px] font-black uppercase px-2 py-1 rounded transition-colors ${
                levelFilter === l
                  ? 'bg-primary/20 text-primary'
                  : 'text-muted-foreground hover:text-foreground'
              }`}
            >
              {l}
            </button>
          ))}
        </div>

        <Button
          variant="ghost" size="sm"
          className="h-7 text-[9px] font-black uppercase gap-1"
          onClick={() => setPaused(p => !p)}
        >
          {paused ? <Play size={11} /> : <Pause size={11} />}
          {paused ? 'Resume' : 'Pause'}
        </Button>
        <Button
          variant="ghost" size="sm"
          className="h-7 text-[9px] font-black uppercase gap-1 text-muted-foreground"
          onClick={() => setLogs([])}
        >
          <Trash2 size={11} /> Flush
        </Button>
      </div>

      {/* Log viewport */}
      <div className="flex-1 overflow-auto bg-[oklch(0.10_0_0)] font-mono text-xs">
        <div className="p-4 space-y-0.5">
          {filtered.length === 0 ? (
            <div className="flex items-center justify-center h-64 text-muted-foreground/30">
              <p className="text-[10px] font-black uppercase tracking-[0.4em]">No log entries</p>
            </div>
          ) : (
            filtered.map((log, i) => (
              <div
                key={i}
                className="flex items-start gap-3 py-1 px-2 rounded hover:bg-white/3 transition-colors group"
              >
                <span className="text-muted-foreground/40 shrink-0 text-[9px] tabular-nums pt-0.5">
                  {log.timestamp.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}
                </span>
                <Badge className={`text-[8px] font-black h-4 px-1.5 shrink-0 border ${levelBadge[log.level]}`}>
                  {log.level.toUpperCase()}
                </Badge>
                <span className={`leading-relaxed break-all ${levelColor[log.level]}`}>
                  {log.raw}
                </span>
              </div>
            ))
          )}
          <div ref={bottomRef} />
        </div>
      </div>
    </div>
  );
};
