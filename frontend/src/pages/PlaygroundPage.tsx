import React, { useState, useRef, useEffect, useCallback } from 'react';
import { Play, Square, Globe, Trash2, Zap, Code2, Bot } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Textarea } from '@/components/ui/textarea';
import { Badge } from '@/components/ui/badge';
import { Separator } from '@/components/ui/separator';
import { ResizablePanelGroup, ResizablePanel, ResizableHandle } from '@/components/ui/resizable';
import { Label } from '@/components/ui/label';
import { Slider } from '@/components/ui/slider';
import { Switch } from '@/components/ui/switch';
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group';
import { toast } from 'sonner';
import sdk, { getOllamaClient, getOpenAIClient } from '../api';
import { useCluster } from '../ClusterContext';
import type { NodeStatus } from '../api';
import { ModelSelector } from '../components/ModelSelector';

type SDKMode = 'flakyollama' | 'ollama' | 'openai';

export const PlaygroundPage: React.FC = () => {
  const { status } = useCluster();
  
  const ollamaClient = React.useMemo(() => getOllamaClient(), []);
  const openaiClient = React.useMemo(() => getOpenAIClient(), []);

  const [sdkMode, setSdkMode] = useState<SDKMode>('ollama');
  const [loading, setLoading] = useState(false);
  const [streamedText, setStreamedText] = useState('');
  const [agentId, setAgentId] = useState<string | null>(null);
  const [selectedModel, setSelectedModel] = useState('');
  const [selectedNode, setSelectedNode] = useState('dynamic');
  const [prompt, setPrompt] = useState('');
  const [temperature, setTemperature] = useState(0.7);
  const [maxTokens, setMaxTokens] = useState(512);
  const [streamEnabled, setStreamEnabled] = useState(true);
  const abortRef = useRef<AbortController | null>(null);
  const responseRef = useRef<HTMLDivElement>(null);

  const nodes = (status ? Object.values(status.nodes) as NodeStatus[] : []);
  const models = status?.all_models || [];

  useEffect(() => {
    if (models.length && !selectedModel) setSelectedModel(models[0]);
  }, [models, selectedModel]);

  // Auto-scroll as tokens stream in
  useEffect(() => {
    if (responseRef.current) {
      responseRef.current.scrollTop = responseRef.current.scrollHeight;
    }
  }, [streamedText]);

  const stop = useCallback(() => {
    abortRef.current?.abort();
    setLoading(false);
  }, []);

  const run = useCallback(async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedModel || !prompt.trim()) return;

    abortRef.current = new AbortController();
    const signal = abortRef.current.signal;
    setLoading(true);
    setStreamedText('');
    setAgentId(null);

    try {
      if (sdkMode === 'flakyollama') {
        // Original FlakyOllama custom endpoint
        const res = await sdk.testInference({
          model: selectedModel,
          prompt: prompt.trim(),
          node_id: selectedNode !== 'dynamic' ? selectedNode : undefined,
        });
        setStreamedText(res.response);
        setAgentId(res.agent_id);

      } else if (sdkMode === 'ollama') {
        // Ollama JS SDK — native streaming via browser client
        const stream = await ollamaClient.generate({
          model: selectedModel,
          prompt: prompt.trim(),
          stream: true,
          options: {
            temperature,
            num_predict: maxTokens,
          },
        });

        for await (const chunk of stream) {
          if (signal.aborted) break;
          setStreamedText(prev => prev + chunk.response);
          if (chunk.done) {
            setAgentId('ollama-sdk');
          }
        }

      } else {
        // OpenAI SDK — OpenAI-compatible streaming
        const nodeHeader = selectedNode !== 'dynamic' ? { 'X-Node-Id': selectedNode } : {};
        if (streamEnabled) {
          const stream = await openaiClient.chat.completions.create({
            model: selectedModel,
            messages: [{ role: 'user', content: prompt.trim() }],
            stream: true,
            temperature,
            max_tokens: maxTokens,
          }, { headers: nodeHeader, signal });

          for await (const chunk of stream) {
            if (signal.aborted) break;
            const delta = chunk.choices[0]?.delta?.content ?? '';
            setStreamedText(prev => prev + delta);
          }
          setAgentId('openai-sdk');
        } else {
          const res = await openaiClient.chat.completions.create({
            model: selectedModel,
            messages: [{ role: 'user', content: prompt.trim() }],
            stream: false,
            temperature,
            max_tokens: maxTokens,
          }, { headers: nodeHeader, signal });
          setStreamedText(res.choices[0]?.message?.content ?? '');
          setAgentId('openai-sdk');
        }
      }
    } catch (err: any) {
      if (err.name === 'AbortError' || signal.aborted) {
        toast.info('Inference stopped');
      } else {
        toast.error(err.message || 'Inference failed');
      }
    } finally {
      setLoading(false);
    }
  }, [sdkMode, selectedModel, selectedNode, prompt, temperature, maxTokens, streamEnabled]);

  const sdkBadgeColors: Record<SDKMode, string> = {
    flakyollama: 'bg-primary/15 text-primary border-primary/30',
    ollama:      'bg-purple-500/15 text-purple-400 border-purple-500/30',
    openai:      'bg-emerald-500/15 text-emerald-400 border-emerald-500/30',
  };
  const sdkLabels: Record<SDKMode, string> = {
    flakyollama: 'FlakyOllama API',
    ollama: 'Ollama SDK',
    openai: 'OpenAI SDK',
  };

  return (
    <div className="h-[calc(100vh-57px)] overflow-hidden">
      {/* @ts-ignore */}
      <ResizablePanelGroup direction="horizontal" className="h-full">
        {/* ── Left: Input Panel ── */}
        <ResizablePanel defaultSize={40} minSize={28} className="flex flex-col">
          <div className="p-5 border-b border-border/50">
            <h2 className="text-sm font-black uppercase tracking-widest mb-3">Inference Playground</h2>

            {/* SDK mode selector */}
            <ToggleGroup
              type="single"
              value={sdkMode}
              onValueChange={v => v && setSdkMode(v as SDKMode)}
              className="w-full gap-1"
            >
              <ToggleGroupItem value="flakyollama" className="flex-1 text-[9px] font-black uppercase tracking-widest h-8 gap-1.5">
                <Zap size={11} /> API
              </ToggleGroupItem>
              <ToggleGroupItem value="ollama" className="flex-1 text-[9px] font-black uppercase tracking-widest h-8 gap-1.5">
                <Bot size={11} /> Ollama
              </ToggleGroupItem>
              <ToggleGroupItem value="openai" className="flex-1 text-[9px] font-black uppercase tracking-widest h-8 gap-1.5">
                <Code2 size={11} /> OpenAI
              </ToggleGroupItem>
            </ToggleGroup>
          </div>

          <div className="flex-1 overflow-y-auto">
            <form id="inference-form" onSubmit={run} className="p-5 space-y-4">
              {/* Model — now shows routability + compat */}
              <div className="space-y-1.5">
                <Label className="text-[9px] font-black uppercase tracking-widest text-muted-foreground">Model</Label>
                <ModelSelector value={selectedModel} onChange={setSelectedModel} sdkMode={sdkMode} />
              </div>

              {/* Node targeting */}
              <div className="space-y-1.5">
                <Label className="text-[9px] font-black uppercase tracking-widest text-muted-foreground">Compute Node</Label>
                <Select value={selectedNode} onValueChange={setSelectedNode}>
                  <SelectTrigger className="bg-muted/50 border-border/50 font-bold text-xs h-9">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="dynamic" className="font-bold text-xs">⚖️ Dynamic Balancing</SelectItem>
                    <Separator className="my-1" />
                    {nodes.map(n => (
                      <SelectItem key={n.id} value={n.id} className="font-bold text-xs">
                        {n.has_gpu ? '⚡' : '●'} {n.id}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              {/* Advanced options — OpenAI + Ollama only */}
              {sdkMode !== 'flakyollama' && (
                <div className="space-y-4 p-3 rounded-lg bg-muted/20 border border-border/30">
                  <p className="text-[9px] font-black uppercase tracking-widest text-muted-foreground">Advanced Options</p>
                  <div className="space-y-1.5">
                    <div className="flex justify-between">
                      <Label className="text-[10px] font-bold text-muted-foreground">Temperature</Label>
                      <span className="text-[10px] font-black text-foreground">{temperature.toFixed(2)}</span>
                    </div>
                    <Slider value={[temperature]} onValueChange={([v]) => setTemperature(v)} min={0} max={2} step={0.01} />
                  </div>
                  <div className="space-y-1.5">
                    <div className="flex justify-between">
                      <Label className="text-[10px] font-bold text-muted-foreground">Max Tokens</Label>
                      <span className="text-[10px] font-black text-foreground">{maxTokens}</span>
                    </div>
                    <Slider value={[maxTokens]} onValueChange={([v]) => setMaxTokens(v)} min={64} max={4096} step={64} />
                  </div>
                  {sdkMode === 'openai' && (
                    <div className="flex items-center justify-between">
                      <Label className="text-[10px] font-bold text-muted-foreground">Streaming</Label>
                      <Switch checked={streamEnabled} onCheckedChange={setStreamEnabled} />
                    </div>
                  )}
                </div>
              )}

              {/* Prompt */}
              <div className="space-y-1.5">
                <Label className="text-[9px] font-black uppercase tracking-widest text-muted-foreground">Prompt</Label>
                <Textarea
                  value={prompt}
                  onChange={e => setPrompt(e.target.value)}
                  placeholder="Enter your prompt..."
                  className="min-h-[160px] bg-muted/50 border-border/50 text-sm font-medium resize-none focus-visible:ring-primary/30"
                  required
                />
              </div>
            </form>
          </div>

          <div className="p-4 border-t border-border/50 flex gap-2">
            {loading ? (
              <Button
                variant="destructive"
                className="flex-1 h-10 font-black text-xs tracking-widest uppercase"
                onClick={stop}
              >
                <Square size={14} className="mr-2" fill="currentColor" /> Stop
              </Button>
            ) : (
              <Button
                type="submit"
                form="inference-form"
                disabled={!selectedModel || !prompt.trim()}
                className="flex-1 h-10 font-black text-xs tracking-widest uppercase shadow-lg shadow-primary/20"
              >
                <Play size={14} className="mr-2" fill="currentColor" fillOpacity={0.3} />
                Execute via {sdkLabels[sdkMode]}
              </Button>
            )}
          </div>
        </ResizablePanel>

        <ResizableHandle withHandle className="bg-border/30" />

        {/* ── Right: Response Terminal ── */}
        <ResizablePanel defaultSize={60} minSize={30} className="flex flex-col bg-[oklch(0.10_0_0)]">
          <div className="px-5 py-3 border-b border-white/5 flex items-center justify-between shrink-0">
            <div className="flex items-center gap-2">
              <div className={`w-2 h-2 rounded-full ${loading ? 'bg-amber-400 animate-pulse' : 'bg-primary'}`} />
              <span className="text-[9px] font-black uppercase tracking-[0.3em] text-muted-foreground">Response Buffer</span>
              {agentId && (
                <Badge className={`text-[8px] font-black h-4 px-2 border ${sdkBadgeColors[sdkMode]}`}>
                  {sdkLabels[sdkMode]} · {agentId}
                </Badge>
              )}
              {loading && (
                <Badge className="text-[8px] font-black h-4 px-2 bg-amber-500/10 text-amber-400 border-amber-500/20 animate-pulse">
                  Streaming...
                </Badge>
              )}
            </div>
            {streamedText && (
              <Button
                variant="ghost" size="sm"
                className="h-6 text-[9px] font-black uppercase text-muted-foreground hover:text-foreground"
                onClick={() => { setStreamedText(''); setAgentId(null); }}
              >
                <Trash2 size={11} className="mr-1" /> Clear
              </Button>
            )}
          </div>
          <div ref={responseRef} className="flex-1 overflow-auto p-6">
            {streamedText ? (
              <pre className="font-mono text-sm text-slate-200 whitespace-pre-wrap leading-relaxed">
                {streamedText}
                {loading && <span className="inline-block w-2 h-4 bg-primary/80 ml-0.5 animate-pulse align-middle" />}
              </pre>
            ) : (
              <div className="h-full flex flex-col items-center justify-center opacity-20 select-none">
                <Globe size={48} className="mb-4 animate-pulse" />
                <p className="text-[10px] font-black uppercase tracking-[0.5em] text-muted-foreground">
                  {loading ? 'Awaiting first token...' : 'Awaiting Transmission'}
                </p>
              </div>
            )}
          </div>
        </ResizablePanel>
      </ResizablePanelGroup>
    </div>
  );
};
