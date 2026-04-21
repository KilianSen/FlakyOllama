import { Ollama } from 'ollama/browser';
import OpenAI from 'openai';

const getBaseUrl = () => localStorage.getItem('BALANCER_URL') || import.meta.env.VITE_BALANCER_URL || '';
const getToken = () => localStorage.getItem('BALANCER_TOKEN') || import.meta.env.VITE_BALANCER_TOKEN || 'your-secret-balancer-token';

export const getOllamaClient = () => {
  const host = getBaseUrl();
  return new Ollama({ host: host || window.location.origin });
};

export const getOpenAIClient = () => {
  const host = getBaseUrl();
  const token = getToken();
  return new OpenAI({
    baseURL: host ? `${host}/v1` : `${window.location.origin}/v1`,
    apiKey: token,
    dangerouslyAllowBrowser: true,
  });
};

export interface ModelInfo {
  name: string;
  modified_at: string;
  size: number;
}

export interface NodeStatus {
  id: string;
  address: string;
  tier: string;
  has_gpu: boolean;
  cpu_usage: number;
  cpu_cores: number;
  memory_usage: number;
  vram_total: number;
  vram_used: number;
  gpu_model: string;
  gpu_temp: number;
  active_models: string[];
  local_models: ModelInfo[];
  last_seen: string;
  state: number;
  errors: number;
  cooloff_until: string;
  draining: boolean;
}

export interface ClusterStatus {
  nodes: Record<string, NodeStatus>;
  pending_requests: Record<string, number>;
  in_progress_pulls: Record<string, string>;
  node_workloads: Record<string, number>;
  queue_depth: number;
  active_workloads: number;
  all_models: string[];
  total_vram: number;
  used_vram: number;
  total_cpu_cores: number;
  avg_cpu_usage: number;
  avg_mem_usage: number;
  uptime_seconds: number;
}

export type JobStatus = 'pending' | 'running' | 'completed' | 'failed';

export interface Job {
  id: string;
  type: string;
  status: JobStatus;
  message?: string;
  progress: number;
  created_at: string;
  updated_at: string;
}

export interface InferenceRequest {
  model: string;
  prompt: string;
  stream?: boolean;
  node_id?: string;
  node_addr?: string;
}

export interface InferenceResponse {
  agent_id: string;
  response: string;
}

export interface RoutingWeights {
  cpu_load_weight: number;
  workload_penalty: number;
  success_rate_weight: number;
  latency_weight: number;
  loaded_model_bonus: number;
  local_model_bonus: number;
}

export interface CBConfig {
  error_threshold: number;
  cooloff_sec: number;
}

export interface TLSConfig {
  enabled: boolean;
  cert_file: string;
  key_file: string;
  insecure_skip_verify: boolean;
}

export interface Config {
  port: number;
  host: string;
  db_path: string;
  auth_token: string;
  remote_token: string;
  max_queue_depth: number;
  enable_hedging: boolean;
  hedging_percentile: number;
  circuit_breaker: CBConfig;
  weights: RoutingWeights;
  stall_timeout_sec: number;
  stale_threshold: number;
  keep_alive_duration_sec: number;
  tls: TLSConfig;
  poll_interval_ms: number;
}

class FlakyOllamaSDK {
  private getHeaders(): Record<string, string> {
    const token = getToken();
    return {
      'Authorization': `Bearer ${token}`,
      'Content-Type': 'application/json',
    };
  }

  private async request<T>(path: string, options: RequestInit = {}): Promise<T> {
    const baseUrl = getBaseUrl();
    const res = await fetch(`${baseUrl}${path}`, {
      ...options,
      headers: { ...this.getHeaders(), ...options.headers },
    });
    
    if (!res.ok) {
      const error = await res.json().catch(() => ({ error: 'Unknown error' }));
      throw new Error(error.error || `Request failed with status ${res.status}`);
    }
    
    if (res.status === 204) return {} as T;
    return res.json();
  }

  // Cluster & Nodes
  async getStatus(): Promise<ClusterStatus> {
    return this.request<ClusterStatus>('/api/v1/status');
  }

  async getNodes(): Promise<NodeStatus[]> {
    return this.request<NodeStatus[]>('/api/v1/nodes');
  }

  async drainNode(id: string): Promise<{ status: string }> {
    return this.request(`/api/v1/nodes/${id}/drain`, { method: 'POST' });
  }

  async undrainNode(id: string): Promise<{ status: string }> {
    return this.request(`/api/v1/nodes/${id}/undrain`, { method: 'POST' });
  }

  // Models
  async pullModel(model: string, nodeId?: string): Promise<{ job_id: string; status: string }> {
    return this.request('/api/v1/models/pull', {
      method: 'POST',
      body: JSON.stringify({ model, node_id: nodeId }),
    });
  }

  async deleteModel(name: string): Promise<{ job_id: string; status: string }> {
    return this.request(`/api/v1/models/${name}`, { method: 'DELETE' });
  }

  async unloadModel(name: string, nodeId?: string): Promise<{ status: string }> {
    return this.request(`/api/v1/models/${name}/unload`, {
      method: 'POST',
      body: JSON.stringify({ node_id: nodeId }),
    });
  }

  // Jobs
  async getJob(id: string): Promise<Job> {
    return this.request<Job>(`/api/v1/jobs/${id}`);
  }

  async waitForJob(id: string, onProgress?: (job: Job) => void): Promise<Job> {
    while (true) {
      const job = await this.getJob(id);
      if (onProgress) onProgress(job);
      
      if (job.status === 'completed') return job;
      if (job.status === 'failed') throw new Error(job.message || 'Job failed');
      
      await new Promise(resolve => setTimeout(resolve, 1000));
    }
  }

  // Inference
  async testInference(req: InferenceRequest): Promise<InferenceResponse> {
    return this.request<InferenceResponse>('/api/v1/test', {
      method: 'POST',
      body: JSON.stringify(req),
    });
  }

  // Logs
  streamLogs(onMessage: (msg: string) => void): () => void {
    const baseUrl = getBaseUrl();
    const token = getToken();
    let logUrl = `${baseUrl}/api/v1/logs`;
    if (!logUrl.startsWith('http')) {
      logUrl = new URL(logUrl, window.location.origin).toString();
    }
    const url = new URL(logUrl);
    url.searchParams.set('token', token);

    let eventSource: EventSource | null = null;
    let retryTimeout: any = null;

    const connect = () => {
      console.log(`[SSE] Connecting to ${url.toString()}...`);
      eventSource = new EventSource(url.toString());
      
      eventSource.onopen = () => {
        console.log('[SSE] Connection established');
      };

      eventSource.onmessage = (event) => {
        console.debug('[SSE] Received message:', event.data);
        onMessage(event.data);
      };

      eventSource.onerror = (err) => {
        console.error('[SSE] EventSource failed:', err);
        if (eventSource) eventSource.close();
        
        // Retry after 3s
        clearTimeout(retryTimeout);
        retryTimeout = setTimeout(connect, 3000);
      };
    };

    connect();

    return () => {
      console.log('[SSE] Closing connection');
      if (eventSource) eventSource.close();
      clearTimeout(retryTimeout);
    };
  }

  // Config
  async getConfig(): Promise<Config> {
    return this.request<Config>('/api/v1/config');
  }

  async updateConfig(config: Config): Promise<{ status: string }> {
    return this.request('/api/v1/config', {
      method: 'POST',
      body: JSON.stringify(config),
    });
  }
}

export const sdk = new FlakyOllamaSDK();
export const api = sdk;
export default sdk;
