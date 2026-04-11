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
  draining: boolean;
}

export interface ClusterStatus {
  nodes: Record<string, NodeStatus>;
  pending_requests: Record<string, number>;
  queue_depth: number;
  active_workloads: number;
  all_models: string[];
}

const headers = {
  'Authorization': `Bearer ${BALANCER_TOKEN}`,
  'Content-Type': 'application/json',
};

export const api = {
  async getStatus(): Promise<ClusterStatus> {
    const res = await fetch(`${API_BASE_URL}/api/status`, { headers });
    if (!res.ok) throw new Error('Failed to fetch status');
    return res.json();
  },

  async drainNode(addr: string): Promise<void> {
    await fetch(`${API_BASE_URL}/api/manage/node/drain?addr=${addr}`, { method: 'POST', headers });
  },

  async undrainNode(addr: string): Promise<void> {
    await fetch(`${API_BASE_URL}/api/manage/node/undrain?addr=${addr}`, { method: 'POST', headers });
  },

  async unloadModel(addr: string, model: string): Promise<void> {
    await fetch(`${API_BASE_URL}/api/manage/model/unload?addr=${addr}&model=${model}`, { method: 'POST', headers });
  },

  async deleteModel(addr: string, model: string): Promise<void> {
    await fetch(`${API_BASE_URL}/api/manage/model/delete?addr=${addr}&model=${model}`, { method: 'POST', headers });
  },

  async pullModel(model: string, addr?: string): Promise<void> {
    const url = addr 
      ? `${API_BASE_URL}/api/manage/model/pull?addr=${addr}&model=${model}`
      : `${API_BASE_URL}/api/manage/model/pull?model=${model}`;
    await fetch(url, { method: 'POST', headers });
  },

  async runTest(model: string, prompt: string): Promise<{agent_id: string, response: string}> {
    const res = await fetch(`${API_BASE_URL}/api/manage/test`, {
      method: 'POST',
      headers,
      body: JSON.stringify({ model, prompt }),
    });
    if (!res.ok) throw new Error('Test failed');
    return res.json();
  },

  async runTestOnNode(model: string, prompt: string, addr: string): Promise<{agent_id: string, response: string}> {
    const res = await fetch(`${API_BASE_URL}/api/manage/test`, {
      method: 'POST',
      headers,
      body: JSON.stringify({ model, prompt, node_addr: addr }),
    });
    if (!res.ok) throw new Error('Test on node failed');
    return res.json();
  },

  streamLogs(onMessage: (msg: string) => void): () => void {
    const eventSource = new EventSource(`${API_BASE_URL}/api/logs`);
    eventSource.onmessage = (event) => {
      onMessage(event.data);
    };
    eventSource.onerror = (err) => {
      console.error('EventSource failed:', err);
      eventSource.close();
    };
    return () => eventSource.close();
  }
};
