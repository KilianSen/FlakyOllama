# FlakyOllama

**FlakyOllama** is a distributed, intelligent orchestration layer for Ollama. It transforms a collection of unreliable or intermittent nodes into a robust, high-performance inference cluster. 

By combining **centralized routing** with **decentralized edge guardrails**, FlakyOllama ensures that every request is handled by the best possible node, while protecting individual nodes from resource exhaustion or performance degradation.

---

## Core Pillars

### 1. Centralized Smart Routing
The Balancer maintains a global view of the cluster, routing requests based on:
- **Hardware Telemetry**: Real-time CPU usage, VRAM availability, and GPU temperature.
- **Historical Performance**: Learned metrics (TPS, TTFT, Success Rate) per model/node pair.
- **Warm-start Optimization**: Prioritizing nodes where the requested model is already resident in memory.
- **Virtual Models**: Grouping multiple backing models into a single "smart" alias (e.g., `smart-fastest` or `auto-grader`).

### 2. Decentralized Edge Guardrails
Agents locally enforce "Model Fitting" policies to protect themselves and the cluster quality:
- **Load Shedding**: Autonomous rejection of requests when local hardware thresholds (CPU/Mem) are breached.
- **Performance Guardrails**: Blocking specific models locally if they exhibit low throughput (TPS), high error rates, or excessive latency.
- **Priority Overrides**: Ensuring critical workloads (high-priority) bypass local load shedding during stress periods.

### 3. Transparent Fault Tolerance
- **Global Request Queue**: Incoming requests are safely buffered when the cluster is at capacity.
- **Automatic Failover**: If a node fails or hangs mid-inference, the Balancer transparently retries the request on a different healthy node.
- **Circuit Breaking**: Flaky nodes are automatically "cooled off" and temporarily removed from routing until they stabilize.

---

## System Architecture

FlakyOllama consists of two main components:

### The Balancer (Central Brain)
- Aggregates telemetry from all Agents.
- Manages the global request queue and priority scheduling.
- Provides a **unified API** (Ollama & OpenAI compatible).
- Handles OIDC authentication and client key management.

### The Agent (Edge Intelligence)
- Runs alongside the local Ollama instance.
- Ships high-frequency hardware and model metrics.
- Acts as an intelligent reverse proxy with local policy enforcement (Edge Guardrails).
- Manages local model lifecycle (pulling, deleting, pre-warming).

---

## Configuration

### Global Environment Variables
| Variable | Description |
| :--- | :--- |
| `ROLE` | `balancer` or `agent`. Default is `balancer`. |
| `CONFIG_PATH` | Path to optional JSON configuration for advanced settings. |

### Balancer Variables
| Variable | Description |
| :--- | :--- |
| `BALANCER_ADDR` | Listen address (default: `0.0.0.0:8080`). |
| `BALANCER_TOKEN` | Master token required for clients (if Auth enabled). |
| `REMOTE_TOKEN` | Token sent to agents for registration. |
| `DB_PATH` | SQLite path for metrics and cluster state. |

### Agent Variables
| Variable | Description |
| :--- | :--- |
| `AGENT_ID` | Unique identifier for this node. |
| `BALANCER_URL` | URL of the central Balancer. |
| `OLLAMA_URL` | URL of the local Ollama instance. |
| `AGENT_TOKEN` | Token the agent uses to register and accepts for incoming cluster calls. |
| **Edge Guardrail Config** | |
| `AGENT_REJECT_ON_HIGH_LOAD` | `true` to enable autonomous load shedding. |
| `AGENT_MAX_CPU_THRESHOLD` | CPU % threshold to start rejecting (default: 90.0). |
| `AGENT_MIN_TPS` | Min Tokens-Per-Second required for a model to be considered healthy. |
| `AGENT_MAX_ERROR_RATE` | Max local error rate (0.0-1.0) before disabling a model. |
| `AGENT_MIN_PRIORITY_UNDER_LOAD` | Minimum priority required to bypass load-based rejection. |

---

## API Reference

FlakyOllama is a drop-in replacement for any Ollama client and offers OpenAI compatibility.

### Inference Endpoints (Balancer)
- `POST /api/chat` / `POST /api/generate` (Ollama Native)
- `POST /v1/chat/completions` / `POST /v1/completions` (OpenAI Compatible)
- `POST /api/embeddings` (Embeddings)

### Management Endpoints
- `GET /api/v1/status` - Full cluster health, node loads, and performance map.
- `GET /api/v1/catalog` - List of all models and their reward/cost factors.
- `GET /capabilities` (Agent Port) - View or update local edge policies.

---

## Example Usage

### Running with Docker Compose
```yaml
services:
  balancer:
    image: flakyollama
    environment: [ ROLE=balancer, BALANCER_TOKEN=secret ]

  agent-gpu:
    image: flakyollama
    environment:
      - ROLE=agent
      - BALANCER_URL=http://balancer:8080
      - AGENT_REJECT_ON_HIGH_LOAD=true
      - AGENT_MIN_TPS=15.0 # Reject slow model fits locally
```

### Sending a Priority Request
```bash
curl http://localhost:8080/api/chat -H "Authorization: Bearer secret" -d '{
  "model": "llama3",
  "messages": [{"role": "user", "content": "Critical task"}],
  "priority": 100
}'
```
