const API_BASE_URL = ''; // Use relative paths for the proxy
const BALANCER_TOKEN = import.meta.env.VITE_BALANCER_TOKEN || 'your-secret-balancer-token';

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
    const eventSource = new EventSource(`${API_BASE_URL}/api/v1/logs`);
    eventSource.onmessage = (event) => {
      onMessage(event.data);
    };
    eventSource.onerror = (err) => {
      console.error('EventSource failed:', err);
      eventSource.close();
    };
    return () => eventSource.close();
  }
}

export const sdk = new FlakyOllamaSDK(BALANCER_TOKEN);
export default sdk;
