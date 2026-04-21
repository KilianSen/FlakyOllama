const API_BASE_URL = localStorage.getItem('BALANCER_URL') || import.meta.env.VITE_BALANCER_URL || ''; 
const BALANCER_TOKEN = localStorage.getItem('BALANCER_TOKEN') || import.meta.env.VITE_BALANCER_TOKEN || 'your-secret-balancer-token';

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
  private headers: Record<string, string>;

  constructor(token: string) {
    this.headers = {
      'Authorization': `Bearer ${token}`,
      'Content-Type': 'application/json',
    };
  }

  private async request<T>(path: string, options: RequestInit = {}): Promise<T> {
    const res = await fetch(`${API_BASE_URL}${path}`, {
      ...options,
      headers: { ...this.headers, ...options.headers },
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
    let logUrl = `${API_BASE_URL}/api/v1/logs`;
    if (!logUrl.startsWith('http')) {
      logUrl = new URL(logUrl, window.location.origin).toString();
    }
    const url = new URL(logUrl);
    url.searchParams.set('token', BALANCER_TOKEN);
    const eventSource = new EventSource(url.toString());
    eventSource.onmessage = (event) => {
      onMessage(event.data);
    };
    eventSource.onerror = (err) => {
      console.error('EventSource failed:', err);
      eventSource.close();
    };
    return () => eventSource.close();
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

export const sdk = new FlakyOllamaSDK(BALANCER_TOKEN);
export const api = sdk;
export default sdk;
