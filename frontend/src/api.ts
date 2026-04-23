import { Ollama } from 'ollama/browser';
import OpenAI from 'openai';

const getBaseUrl = () => {
  return ''; // Always use relative paths to go through Vite proxy / Nginx
};

const getToken = () => localStorage.getItem('BALANCER_TOKEN') || import.meta.env.VITE_BALANCER_TOKEN || '';

export interface NodeStatus {
  id: string;
  address: string;
  state: number;
  tier: string;
  cpu_usage: number;
  cpu_cores: number;
  memory_usage: number;
  memory_total: number;
  vram_total: number;
  vram_used: number;
  gpu_model: string;
  gpu_temp: number;
  active_models: string[];
  local_models: Array<{ model: string, size: number }>;
  input_tokens: number;
  output_tokens: number;
  token_reward: number;
  reputation: number;
  errors: number;
  message: string;
  has_gpu: boolean;
  draining: boolean;
  last_seen: string;
  agent_key?: string;
}

export interface ClusterStatus {
  nodes: Record<string, NodeStatus>;
  active_workloads: number;
  avg_cpu_usage: number;
  avg_mem_usage: number;
  pending_requests: Record<string, number>;
  all_models: string[];
  performance: Record<string, { avg_ttft_ms: number, avg_duration_ms: number, requests: number }>;
  total_vram: number;
  used_vram: number;
  uptime_seconds: number;
  queue_depth: number;
  total_cpu_cores: number;
  total_input_tokens: number;
  total_output_tokens: number;
  total_reward: number;
  total_cost: number;
  node_workloads: Record<string, number>;
  in_progress_pulls: Record<string, string>;
  model_policies: Record<string, Record<string, { banned: boolean, pinned: boolean }>>;
}

export interface ClientKey {
  key: string;
  label: string;
  quota_limit: number;
  quota_used: number;
  credits: number;
  active: boolean;
  user_id?: string;
}

export interface AgentKey {
  key: string;
  label: string;
  node_id: string;
  credits_earned: number;
  reputation: number;
  active: boolean;
  user_id?: string;
}

export interface LogEntry {
  timestamp: string;
  node_id: string;
  level: string;
  component: string;
  message: string;
}

export interface ModelRequest {
  id: string;
  type: string;
  model: string;
  node_id: string;
  status: string;
  requested_at: string;
}

export interface Catalog {
  global_reward_multiplier: number;
  global_cost_multiplier: number;
  models: Array<{ name: string, reward_factor: number, cost_factor: number }>;
}

export interface ProfileResponse {
  user: User;
  client_key: ClientKey;
  agent_keys: AgentKey[];
}

export interface User {
  id: string;
  sub: string;
  email: string;
  name: string;
  is_admin: boolean;
}

export interface UserWithKey {
  user: User;
  key: ClientKey;
}

export type Identity = {
    type: 'client' | 'agent';
    label: string;
    data: any;
};

class FlakyOllamaSDK {
  private async request<T>(path: string, options: RequestInit = {}, tokenOverride?: string): Promise<T> {
    const baseUrl = getBaseUrl();
    const token = tokenOverride || getToken();
    const res = await fetch(`${baseUrl}${path}`, {
      ...options,
      credentials: 'include',
      headers: { 
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json',
        ...options.headers 
      },
    });

    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: res.statusText }));
      throw new Error(err.error || `HTTP ${res.status}`);
    }

    if (res.status === 204) return {} as T;
    return res.json();
  }

  // Legacy/Compatibility
  async getStatus(): Promise<ClusterStatus> {
    return this.getClusterStatus();
  }

  async getClusterStatus(): Promise<ClusterStatus> {
    return this.request<ClusterStatus>('/api/v1/status');
  }

  async streamLogs(): Promise<ReadableStream> {
      return this.getLogs();
  }

  async getLogs(): Promise<ReadableStream> {
    const baseUrl = getBaseUrl();
    const res = await fetch(`${baseUrl}/api/v1/logs`, {
      headers: { 'Authorization': `Bearer ${getToken()}` },
    });
    return res.body!;
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

  async pullModel(name: string, nodeId?: string): Promise<{ status: string, job_id: string }> {
    return this.request<{ status: string, job_id: string }>('/api/v1/models/pull', {
      method: 'POST',
      body: JSON.stringify({ model: name, node_id: nodeId }),
    });
  }

  async deleteModel(name: string): Promise<{ status: string }> {
    return this.request(`/api/v1/models/${name}`, { method: 'DELETE' });
  }

  async unloadModel(name: string, nodeId?: string): Promise<{ status: string }> {
    return this.request(`/api/v1/models/${name}/unload`, {
      method: 'POST',
      body: JSON.stringify({ node_id: nodeId }),
    });
  }

  // Jobs
  async getJobStatus(id: string): Promise<{ status: string, progress: number, message?: string }> {
    return this.request(`/api/v1/jobs/${id}`);
  }

  async waitForJob(id: string): Promise<void> {
    return new Promise((resolve, reject) => {
      const check = async () => {
        try {
          const res = await this.getJobStatus(id);
          if (res.status === 'completed') resolve();
          else if (res.status === 'failed') reject(new Error(res.message || 'Job failed'));
          else setTimeout(check, 1000);
        } catch (err) { reject(err); }
      };
      check();
    });
  }

  // Model Requests
  async getModelRequests(): Promise<ModelRequest[]> {
    return this.request<ModelRequest[]>('/api/v1/requests');
  }

  async approveModelRequest(id: string): Promise<{ status: string }> {
      return this.approveRequest(id);
  }

  async approveRequest(id: string): Promise<{ status: string }> {
    return this.request(`/api/v1/requests/${id}/approve`, { method: 'POST' });
  }

  async declineModelRequest(id: string): Promise<{ status: string }> {
      return this.declineRequest(id);
  }

  async declineRequest(id: string): Promise<{ status: string }> {
    return this.request(`/api/v1/requests/${id}/decline`, { method: 'POST' });
  }

  // Configuration
  async getConfig(): Promise<any> {
    return this.request('/api/v1/config');
  }

  async updateConfig(cfg: any): Promise<{ status: string }> {
    return this.request('/api/v1/config', {
      method: 'POST',
      body: JSON.stringify(cfg),
    });
  }

  // Key Management
  async getClientKeys(): Promise<ClientKey[]> {
    return this.request<ClientKey[]>('/api/v1/keys/clients');
  }

  async createClientKey(k: Partial<ClientKey>): Promise<ClientKey> {
    return this.request<ClientKey>('/api/v1/keys/clients', {
      method: 'POST',
      body: JSON.stringify(k),
    });
  }

  async getAgentKeys(): Promise<AgentKey[]> {
    return this.request<AgentKey[]>('/api/v1/keys/agents');
  }

  async createAgentKey(k: Partial<AgentKey>): Promise<AgentKey> {
    return this.request<AgentKey>('/api/v1/keys/agents', {
      method: 'POST',
      body: JSON.stringify(k),
    });
  }

  // Policies
  async setModelPolicy(model: string, nodeId: string, banned: boolean, pinned: boolean): Promise<{status: string}> {
      return this.request('/api/v1/policies', {
          method: 'POST',
          body: JSON.stringify({ model, node_id: nodeId, is_banned: banned, is_pinned: pinned })
      });
  }

  // Public / Self-service
  async getCatalog(): Promise<Catalog> {
    return this.request<Catalog>('/api/v1/catalog');
  }

  async getMe(): Promise<ProfileResponse> {
    return this.request<ProfileResponse>('/api/v1/me');
  }

  // User Management (Admin)
  async getUsers(): Promise<UserWithKey[]> {
    return this.request<UserWithKey[]>('/api/v1/users');
  }

  async updateUserQuota(userId: string, quota: number): Promise<{ status: string }> {
    return this.request(`/api/v1/users/${userId}/quota`, {
      method: 'POST',
      body: JSON.stringify({ quota_limit: quota }),
    });
  }

  async testInference(req: any): Promise<any> {
      return this.request('/api/v1/test', {
          method: 'POST',
          body: JSON.stringify(req)
      });
  }

  getOllamaClient() {
    return new Ollama({ host: getBaseUrl() || window.location.origin });
  }

  getOpenAIClient() {
    return new OpenAI({ 
        baseURL: (getBaseUrl() || window.location.origin) + '/v1', 
        apiKey: getToken(),
        dangerouslyAllowBrowser: true 
    });
  }
}

export const sdk = new FlakyOllamaSDK();
export const api = sdk;
export default sdk;
