import React, { useState, useRef, useEffect, useCallback } from 'react';
import { Send, Square, Trash2, Bot, Code2, ChevronDown, User2, Copy, Check } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Textarea } from '@/components/ui/textarea';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Badge } from '@/components/ui/badge';
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group';
import { Slider } from '@/components/ui/slider';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';
import { toast } from 'sonner';
import OpenAI from 'openai';
import { getOllamaClient, getOpenAIClient } from '../api';
import { useCluster } from '../ClusterContext';
import {
  computeRoutability, LATENCY_HINTS,
} from '../lib/modelUtils';

type Role = 'user' | 'assistant' | 'system';
type SDKMode = 'ollama' | 'openai';

interface Message {
  id: string;
  role: Role;
  content: string;
  streaming?: boolean;
  sdk?: SDKMode;
}

function MessageBubble({ msg }: { msg: Message }) {
  const [copied, setCopied] = useState(false);
  const isUser = msg.role === 'user';

  const copy = () => {
    navigator.clipboard.writeText(msg.content);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };

  return (
    <div className={`flex gap-3 group ${isUser ? 'flex-row-reverse' : 'flex-row'}`}>
      {/* Avatar */}
      <div className={`w-7 h-7 rounded-lg flex items-center justify-center shrink-0 mt-0.5 ${
        isUser ? 'bg-primary/20' : 'bg-muted'
      }`}>
        {isUser
          ? <User2 size={14} className="text-primary" />
          : <Bot size={14} className="text-muted-foreground" />
        }
      </div>

      {/* Bubble */}
      <div className={`relative max-w-[78%] flex flex-col gap-1 ${isUser ? 'items-end' : 'items-start'}`}>
        <div className={`px-4 py-3 rounded-2xl text-sm leading-relaxed ${
          isUser
            ? 'bg-primary text-primary-foreground rounded-tr-sm'
            : 'bg-muted/60 text-foreground rounded-tl-sm border border-border/30'
        }`}>
          {msg.streaming && !msg.content ? (
            <span className="flex gap-1.5 py-1">
              <span className="w-1.5 h-1.5 rounded-full bg-current animate-bounce [animation-delay:0ms]" />
              <span className="w-1.5 h-1.5 rounded-full bg-current animate-bounce [animation-delay:150ms]" />
              <span className="w-1.5 h-1.5 rounded-full bg-current animate-bounce [animation-delay:300ms]" />
            </span>
          ) : (
            <pre className="font-sans whitespace-pre-wrap break-words text-sm leading-relaxed">
              {msg.content}
              {msg.streaming && <span className="inline-block w-0.5 h-4 bg-current ml-0.5 animate-pulse align-middle" />}
            </pre>
          )}
        </div>

        {/* Copy button */}
        {msg.content && !msg.streaming && (
          <button
            onClick={copy}
            className="opacity-0 group-hover:opacity-100 transition-opacity text-muted-foreground hover:text-foreground"
          >
            {copied ? <Check size={11} className="text-emerald-400" /> : <Copy size={11} />}
          </button>
        )}
        {msg.sdk && (
          <span className="text-[8px] font-bold text-muted-foreground/50 uppercase tracking-widest">
            via {msg.sdk === 'openai' ? 'OpenAI SDK' : 'Ollama SDK'}
          </span>
        )}
      </div>
    </div>
  );
}

export const ChatPage: React.FC = () => {
  const { status } = useCluster();

  const ollamaClient = React.useMemo(() => getOllamaClient(), []);
  const openaiClient = React.useMemo(() => getOpenAIClient(), []);

  const [sdkMode, setSdkMode] = useState<SDKMode>('openai');
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState('');
  const [loading, setLoading] = useState(false);
  const [selectedModel, setSelectedModel] = useState('');
  const [temperature, setTemperature] = useState(0.7);
  const [systemPrompt, setSystemPrompt] = useState('You are a helpful assistant.');
  const [settingsOpen, setSettingsOpen] = useState(false);
  const abortRef = useRef<AbortController | null>(null);
  const bottomRef = useRef<HTMLDivElement>(null);

  const models = status?.all_models || [];

  useEffect(() => {
    if (models.length && !selectedModel) setSelectedModel(models[0]);
  }, [models, selectedModel]);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  const stop = useCallback(() => {
    abortRef.current?.abort();
    setLoading(false);
    setMessages(prev => prev.map(m => m.streaming ? { ...m, streaming: false } : m));
  }, []);

  const send = useCallback(async () => {
    if (!input.trim() || !selectedModel || loading) return;

    const userMsg: Message = {
      id: crypto.randomUUID(),
      role: 'user',
      content: input.trim(),
    };
    const assistantId = crypto.randomUUID();
    const assistantMsg: Message = {
      id: assistantId,
      role: 'assistant',
      content: '',
      streaming: true,
      sdk: sdkMode,
    };

    setMessages(prev => [...prev, userMsg, assistantMsg]);
    setInput('');
    setLoading(true);
    abortRef.current = new AbortController();
    const signal = abortRef.current.signal;

    // Build history for context
    const history = [...messages, userMsg];

    try {
      if (sdkMode === 'openai') {
        const openaiMessages: OpenAI.ChatCompletionMessageParam[] = [
          { role: 'system', content: systemPrompt },
          ...history.map(m => ({
            role: m.role as 'user' | 'assistant',
            content: m.content,
          })),
        ];

        const stream = await openaiClient.chat.completions.create({
          model: selectedModel,
          messages: openaiMessages,
          stream: true,
          temperature,
        }, { signal });

        let accumulated = '';
        for await (const chunk of stream) {
          if (signal.aborted) break;
          const delta = chunk.choices[0]?.delta?.content ?? '';
          accumulated += delta;
          setMessages(prev => prev.map(m =>
            m.id === assistantId ? { ...m, content: accumulated } : m
          ));
        }
        setMessages(prev => prev.map(m =>
          m.id === assistantId ? { ...m, streaming: false } : m
        ));

      } else {
        // Ollama SDK chat endpoint
        const ollamaMessages = [
          { role: 'system' as const, content: systemPrompt },
          ...history.map(m => ({
            role: m.role as 'user' | 'assistant',
            content: m.content,
          })),
        ];

        const stream = await ollamaClient.chat({
          model: selectedModel,
          messages: ollamaMessages,
          stream: true,
          options: { temperature },
        });

        let accumulated = '';
        for await (const chunk of stream) {
          if (signal.aborted) break;
          accumulated += chunk.message.content;
          setMessages(prev => prev.map(m =>
            m.id === assistantId ? { ...m, content: accumulated } : m
          ));
        }
        setMessages(prev => prev.map(m =>
          m.id === assistantId ? { ...m, streaming: false } : m
        ));
      }
    } catch (err: any) {
      if (err.name === 'AbortError' || signal.aborted) {
        toast.info('Generation stopped');
        setMessages(prev => prev.map(m =>
          m.id === assistantId ? { ...m, streaming: false } : m
        ));
      } else {
        toast.error(err.message || 'Inference failed');
        setMessages(prev => prev.filter(m => m.id !== assistantId));
      }
    } finally {
      setLoading(false);
    }
  }, [input, selectedModel, loading, sdkMode, messages, systemPrompt, temperature]);

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      send();
    }
  };

  return (
    <div className="h-[calc(100vh-57px)] flex flex-col">
      {/* Chat header */}
      <div className="px-6 py-3 border-b border-border/50 flex items-center gap-4 shrink-0">
        {/* SDK toggle */}
        <ToggleGroup type="single" value={sdkMode} onValueChange={v => v && setSdkMode(v as SDKMode)} className="gap-1">
          <ToggleGroupItem value="openai" className="text-[9px] font-black uppercase tracking-widest h-7 px-3 gap-1.5">
            <Code2 size={10} /> OpenAI SDK
          </ToggleGroupItem>
          <ToggleGroupItem value="ollama" className="text-[9px] font-black uppercase tracking-widest h-7 px-3 gap-1.5">
            <Bot size={10} /> Ollama SDK
          </ToggleGroupItem>
        </ToggleGroup>

        {/* Model selector with routability */}
        <div className="relative w-64">
          <Select value={selectedModel} onValueChange={setSelectedModel}>
            <SelectTrigger className="h-7 w-full bg-muted/50 border-border/50 font-bold text-[10px] uppercase tracking-wider">
              <SelectValue placeholder="Model..." />
            </SelectTrigger>
            <SelectContent>
              {models.map(m => {
                const r = status ? computeRoutability(m, status) : null;
                const hint = r ? LATENCY_HINTS[r.latencyHint] : null;
                return (
                  <SelectItem key={m} value={m} className="font-bold text-xs py-1.5">
                    <div className="flex items-center gap-2">
                       <span className="truncate">{m}</span>
                       {hint && <span className={`text-[8px] font-black uppercase shrink-0 ${hint.color}`}>{hint.label}</span>}
                    </div>
                  </SelectItem>
                );
              })}
            </SelectContent>
          </Select>
        </div>

        {messages.length > 0 && (
          <Badge variant="outline" className="text-[9px] font-black h-5">
            {messages.filter(m => m.role !== 'system').length} messages
          </Badge>
        )}

        <div className="ml-auto flex items-center gap-2">
          {loading && (
            <Badge className="text-[9px] font-black bg-amber-500/10 text-amber-400 border-amber-500/20 animate-pulse">
              Streaming...
            </Badge>
          )}
          {messages.length > 0 && (
            <Button
              variant="ghost" size="sm"
              className="h-7 text-[9px] font-black uppercase gap-1 text-muted-foreground"
              onClick={() => setMessages([])}
            >
              <Trash2 size={11} /> Clear
            </Button>
          )}
        </div>
      </div>

      {/* Settings panel */}
      <Collapsible open={settingsOpen} onOpenChange={setSettingsOpen}>
        <CollapsibleTrigger className="w-full px-6 py-2 flex items-center gap-2 text-[9px] font-black uppercase tracking-widest text-muted-foreground border-b border-border/30 hover:bg-muted/10 transition-colors">
          <ChevronDown size={11} className={`transition-transform ${settingsOpen ? 'rotate-180' : ''}`} />
          Session Settings — Temperature {temperature.toFixed(2)}
        </CollapsibleTrigger>
        <CollapsibleContent>
          <div className="px-6 py-4 bg-muted/10 border-b border-border/30 grid grid-cols-2 gap-6">
            <div className="space-y-2">
              <div className="flex justify-between text-[10px] font-bold text-muted-foreground">
                <span>Temperature</span><span className="text-foreground">{temperature.toFixed(2)}</span>
              </div>
              <Slider value={[temperature]} onValueChange={([v]) => setTemperature(v)} min={0} max={2} step={0.01} />
            </div>
            <div className="space-y-2">
              <p className="text-[10px] font-bold text-muted-foreground">System Prompt</p>
              <Textarea
                value={systemPrompt}
                onChange={e => setSystemPrompt(e.target.value)}
                className="h-16 text-xs bg-muted/50 border-border/50 resize-none"
                placeholder="System prompt..."
              />
            </div>
          </div>
        </CollapsibleContent>
      </Collapsible>

      {/* Messages */}
      <ScrollArea className="flex-1 px-6 py-6">
        {messages.length === 0 ? (
          <div className="h-full flex flex-col items-center justify-center opacity-30 select-none mt-20">
            <Bot size={56} className="mb-4" />
            <p className="text-[11px] font-black uppercase tracking-[0.4em] text-muted-foreground">Start a conversation</p>
            <p className="text-[10px] text-muted-foreground/60 mt-2">
              Using {sdkMode === 'openai' ? 'OpenAI-compatible SDK' : 'Ollama SDK'} → balancer → agents
            </p>
          </div>
        ) : (
          <div className="space-y-6 max-w-3xl mx-auto">
            {messages.filter(m => m.role !== 'system').map(msg => (
              <MessageBubble key={msg.id} msg={msg} />
            ))}
            <div ref={bottomRef} />
          </div>
        )}
      </ScrollArea>

      {/* Input bar */}
      <div className="px-6 py-4 border-t border-border/50 bg-background/80 backdrop-blur-sm shrink-0">
        <div className="max-w-3xl mx-auto flex gap-3">
          <Textarea
            value={input}
            onChange={e => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Send a message... (Enter to send, Shift+Enter for newline)"
            className="flex-1 min-h-[44px] max-h-[160px] bg-muted/50 border-border/50 text-sm font-medium resize-none focus-visible:ring-primary/30"
            rows={1}
          />
          {loading ? (
            <Button
              variant="destructive" size="icon"
              className="h-11 w-11 shrink-0"
              onClick={stop}
            >
              <Square size={14} fill="currentColor" />
            </Button>
          ) : (
            <Button
              size="icon"
              className="h-11 w-11 shrink-0 shadow-lg shadow-primary/20"
              onClick={send}
              disabled={!input.trim() || !selectedModel}
            >
              <Send size={14} />
            </Button>
          )}
        </div>
        <p className="text-center text-[9px] text-muted-foreground/40 font-bold uppercase tracking-widest mt-2">
          Routed via FlakyOllama balancer · {sdkMode === 'openai' ? 'OpenAI SDK' : 'Ollama SDK'} · {selectedModel || 'no model'}
        </p>
      </div>
    </div>
  );
};
