import type { ClusterStatus, NodeStatus } from '../api';

// ─── Model Capability Inference ──────────────────────────────────────────────

export type ModelCapability = 'chat' | 'generate' | 'embed' | 'vision' | 'code' | 'reasoning';

const EMBEDDING_PATTERNS = ['embed', 'nomic-embed', 'mxbai-embed', 'all-minilm', 'bge-'];
const VISION_PATTERNS    = ['llava', 'bakllava', 'moondream', 'minicpm-v', 'vision', 'cogvlm', 'idefics'];
const CODE_PATTERNS      = ['code', 'coder', 'starcoder', 'codellama', 'deepseek-coder', 'qwen2.5-coder', 'devstral'];
const REASONING_PATTERNS = ['r1', 'qwq', 'deepseek-r', 'o1', 'reason', 'think'];

function matchesAny(name: string, patterns: string[]): boolean {
  return patterns.some(p => name.includes(p));
}

export function inferCapabilities(modelName: string): ModelCapability[] {
  const name = modelName.toLowerCase().split(':')[0]; // strip tag

  if (matchesAny(name, EMBEDDING_PATTERNS)) return ['embed'];

  const caps: ModelCapability[] = [];
  if (matchesAny(name, VISION_PATTERNS))    caps.push('vision');
  if (matchesAny(name, CODE_PATTERNS))      caps.push('code');
  if (matchesAny(name, REASONING_PATTERNS)) caps.push('reasoning');

  // Chat vs base generate
  if (name.includes('instruct') || name.includes('chat') || name.includes('it')) {
    caps.push('chat');
  } else {
    caps.push('generate');
  }

  return caps;
}

// ─── SDK Compatibility ────────────────────────────────────────────────────────

export interface SDKCompat {
  /** Ollama native /api/generate or /api/chat */
  ollamaNative: boolean;
  /** Ollama /api/embeddings */
  ollamaEmbed: boolean;
  /** OpenAI-compatible /v1/chat/completions */
  openAIChat: boolean;
  /** OpenAI-compatible /v1/embeddings */
  openAIEmbed: boolean;
  /** Warning message to show when using OpenAI SDK */
  openAIWarning?: string;
}

export function inferSDKCompat(caps: ModelCapability[]): SDKCompat {
  const isEmbedOnly = caps.includes('embed');
  const hasReasoning = caps.includes('reasoning');

  return {
    ollamaNative: true, // Ollama SDK handles everything
    ollamaEmbed: isEmbedOnly,
    openAIChat: !isEmbedOnly,
    openAIEmbed: isEmbedOnly,
    openAIWarning: isEmbedOnly
      ? 'This is an embedding model — use /v1/embeddings, not /v1/chat/completions'
      : hasReasoning
        ? 'Reasoning models may produce <think> tags not rendered by default'
        : undefined,
  };
}

// ─── Routability ──────────────────────────────────────────────────────────────

export type NodeThermalState = 'hot' | 'warm' | 'cold';

export interface NodeResidency {
  node: NodeStatus;
  thermal: NodeThermalState;
  size?: number; // bytes, if warm (from local_models)
}

export interface ModelRoutability {
  model: string;
  residency: NodeResidency[];
  hotCount:  number;
  warmCount: number;
  coldCount: number;
  totalNodes: number;
  routable: boolean;
  syncing: boolean;
  /** Best-case latency description */
  latencyHint: 'instant' | 'cold-start' | 'unavailable';
}

export function computeRoutability(modelName: string, status: ClusterStatus): ModelRoutability {
  const nodes = Object.values(status.nodes) as NodeStatus[];
  const syncing = !!(status.in_progress_pulls?.[modelName]);

  const residency: NodeResidency[] = nodes.map(node => {
    const isHot  = node.active_models?.includes(modelName) ?? false;
    const warmInfo = node.local_models?.find(lm => lm.name === modelName);
    const thermal: NodeThermalState = isHot ? 'hot' : warmInfo ? 'warm' : 'cold';
    return { node, thermal, size: warmInfo?.size };
  });

  const hotCount  = residency.filter(r => r.thermal === 'hot').length;
  const warmCount = residency.filter(r => r.thermal === 'warm').length;
  const coldCount = residency.filter(r => r.thermal === 'cold').length;
  const routable  = hotCount + warmCount > 0;

  const latencyHint: ModelRoutability['latencyHint'] =
    hotCount > 0 ? 'instant' :
    warmCount > 0 ? 'cold-start' :
    'unavailable';

  return {
    model: modelName,
    residency,
    hotCount,
    warmCount,
    coldCount,
    totalNodes: nodes.length,
    routable,
    syncing,
    latencyHint,
  };
}

// ─── Display Helpers ──────────────────────────────────────────────────────────

export const CAPABILITY_LABELS: Record<ModelCapability, { label: string; color: string; icon: string }> = {
  chat:      { label: 'Chat',      color: 'bg-blue-500/15 text-blue-400 border-blue-500/25',    icon: '💬' },
  generate:  { label: 'Generate',  color: 'bg-slate-500/15 text-slate-400 border-slate-500/25', icon: '✍️' },
  embed:     { label: 'Embedding', color: 'bg-teal-500/15 text-teal-400 border-teal-500/25',   icon: '🔢' },
  vision:    { label: 'Vision',    color: 'bg-pink-500/15 text-pink-400 border-pink-500/25',    icon: '👁️' },
  code:      { label: 'Code',      color: 'bg-emerald-500/15 text-emerald-400 border-emerald-500/25', icon: '⌨️' },
  reasoning: { label: 'Reasoning', color: 'bg-purple-500/15 text-purple-400 border-purple-500/25', icon: '🧠' },
};

export const LATENCY_HINTS = {
  instant:     { label: '⚡ Hot',       color: 'text-emerald-400' },
  'cold-start':{ label: '💾 Warm',     color: 'text-amber-400'   },
  unavailable: { label: '✗ No route', color: 'text-red-400'     },
};

export function formatBytes(bytes: number): string {
  if (!bytes) return '—';
  const gb = bytes / 1e9;
  return gb >= 1 ? `${gb.toFixed(1)} GB` : `${(bytes / 1e6).toFixed(0)} MB`;
}

/** Parse a model tag into family + variant, e.g. "llama3.2:8b" → { family: "llama3.2", variant: "8b" } */
export function parseModelTag(tag: string) {
  const [family, variant = 'latest'] = tag.split(':');
  return { family, variant };
}
