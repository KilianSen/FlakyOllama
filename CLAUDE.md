# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**FlakyOllama** is a distributed orchestration and load-balancing layer for [Ollama](https://ollama.com). It transforms a fleet of individual Ollama nodes (Agents) into a high-availability inference cluster managed by a central coordinator (Balancer).

### Core Components
- **Balancer (Central Brain):** Manages cluster state, routes requests, handles authentication (API keys/OIDC), and executes multi-model pipelines.
- **Agent (Edge Intelligence):** A sidecar for Ollama that reports hardware telemetry, buffers logs in a local SQLite queue, and autonomously enforces "Edge Guardrails."
- **Frontend:** A React 19 dashboard using `ClusterContext` for real-time state polling and cluster management.

---

## Complete File Structure

### Backend implementation (`pkg/`)

- `main.go`: Unified entry point. Switches between `balancer` and `agent` modes via the `ROLE` environment variable.
- `pkg/balancer/`: The central coordinator (the "Brain").
    - `balancer.go`: Initializes the `Balancer` struct and registers all HTTP routes (Ollama, OpenAI, Management).
    - `routing.go`: Implementation of the `SelectAgent` scoring algorithm and virtual model resolution.
    - `state.go`: Manages the global cluster state (`Actor` pattern) with copy-on-read snapshots.
    - `storage.go`: SQLite implementation for persisting metrics, logs, users, and keys (uses WAL mode).
    - `handlers_ollama.go` & `handlers_mgmt.go`: Logic for handling external API requests.
    - `proxy.go`: Intelligent reverse proxy that tracks active workloads and handles request forwarding.
    - `hedging.go`: Implements speculative "hedged" requests to reduce P99 latency.
    - `pipeline.go`: Manages sequential execution of multi-model virtual pipelines.
    - `stall.go`: Detects and recovers from stalled or hanging inference requests.
    - `queue/priority_queue.go`: Min-heap based request prioritization logic.
    - `adapters/openai/`:
        - `openai.go`: Translates OpenAI Chat/Completion requests to Ollama format.
        - `handlers.go`: HTTP handlers for OpenAI-compatible endpoints.
    - `config/config.go`: JSON configuration structures and defaults.
    - `config/persistence.go`: Handles loading/saving configuration to disk.
- `pkg/agent/`: The sidecar running alongside Ollama (the "Edge").
    - `agent.go`: Reverse proxy to the local Ollama API; captures TTFT and token usage metrics.
    - `monitoring/monitor.go`: Polling logic for CPU, RAM, and NVIDIA GPU telemetry (via NVML).
    - `capabilities/manager.go`: Enforces local edge policies like load shedding and model blacklisting.
    - `logging/disk_queue.go`: Buffers structured logs in a local SQLite database before shipping.
    - `ollama/client.go`: High-level Go client for interacting with the local Ollama service.
    - `tasks/manager.go`: Tracks long-running operations like model pulls and creates.
- `pkg/shared/`: Core types and utilities used by both roles.
    - `models/`: Unified data structures for `NodeStatus`, `ClusterStatus`, and `Request`.
    - `auth/auth.go`: Shared Bearer token middleware for agent-balancer and client-balancer auth.
    - `logging/logger.go`: Global structured logging interface with support for pluggable sinks.

### Frontend implementation (`frontend/`)

- `frontend/src/api.ts`: Typed SDK for backend communication.
- `frontend/src/ClusterContext.tsx`: React context for real-time cluster state polling.
- `frontend/src/pages/`:
    - `FleetPage.tsx`: Real-time node monitoring and management.
    - `ChatPage.tsx`: Interactive chat interface.
    - `ConfigPage.tsx`: Cluster-wide settings.
    - `LogsPage.tsx`: Centralized log viewer.
    - `UsersPage.tsx`: Admin-only user and quota management.
    - `KeysPage.tsx`: API key management for users and agents.
- `frontend/src/components/ui/`: Extensive library of shadcn/ui components (30+ components for a rich UI).

---

## Environment Variables

### Core Configuration
| Variable | Default | Purpose |
| :--- | :--- | :--- |
| `ROLE` | `balancer` | Switches binary mode between `balancer` and `agent`. |
| `CONFIG_PATH` | (none) | Path to an optional JSON configuration file. |
| `DB_PATH` | `flakyollama.db`| Path to the SQLite database (Balancer & Agent log buffer). |

### Balancer Settings
| Variable | Default | Purpose |
| :--- | :--- | :--- |
| `BALANCER_ADDR` | `0.0.0.0:8080`| Listen address for the Balancer. |
| `BALANCER_TOKEN` | (none) | Master token required for client authentication. |
| `AGENT_TOKEN` | (none) | Token required for agents to register with the balancer. |

### Agent Settings
| Variable | Default | Purpose |
| :--- | :--- | :--- |
| `AGENT_ID` | (hostname) | Unique identifier for the agent node. |
| `AGENT_ADDR` | `0.0.0.0:8081`| Listen address for the Agent sidecar. |
| `AGENT_ADDRESS` | (none) | Override address advertised to the balancer (useful for NAT/Docker). |
| `AGENT_TIER` | `dedicated` | Tier designation for the node (`shared` or `dedicated`). |
| `BALANCER_URL` | (none) | URL of the central Balancer for registration and telemetry. |
| `OLLAMA_URL` | `http://localhost:11434`| URL of the local Ollama instance. |
| `AGENT_TOKEN` | (none) | Token used by the agent to authenticate with the balancer. |
| `AGENT_AUTH_TOKEN` | (none) | Specific token for direct admin access to the agent. |
| `AGENT_AUTH_TOKEN_DISABLE` | `false` | If `true`, disables authentication on the agent port (INSECURE). |

### Agent Edge Guardrails (Manager)
| Variable | Default | Purpose |
| :--- | :--- | :--- |
| `AGENT_ALLOWED_MODELS` | (none) | Comma-separated list of models the agent is allowed to run. |
| `AGENT_DENY_MODELS` | (none) | Comma-separated list of models to explicitly block. |
| `AGENT_MODEL_PRIORITIES` | (none) | Key-value pairs (model=priority) for local prioritization. |
| `AGENT_REJECT_ON_HIGH_LOAD` | `false` | Enable autonomous load shedding. |
| `AGENT_MAX_CPU_THRESHOLD` | `90.0` | CPU % threshold to start rejecting requests. |
| `AGENT_MAX_MEM_THRESHOLD` | `90.0` | Memory % threshold to start rejecting requests. |
| `AGENT_MIN_PRIORITY_UNDER_LOAD`| `0` | Minimum request priority to bypass load shedding. |
| `AGENT_MIN_TPS` | `0.0` | Minimum Tokens-Per-Second required for a model to be active. |
| `AGENT_MAX_ERROR_RATE` | `1.0` | Maximum local error rate (0-1) before disabling a model. |
| `AGENT_MAX_P95_LATENCY` | `0.0` | Maximum P95 latency (seconds) before disabling a model locally. |

### OIDC & Authentication
| Variable | Default | Purpose |
| :--- | :--- | :--- |
| `OIDC_ENABLED` | `false` | Enable OIDC-based authentication for the dashboard. |
| `OIDC_ISSUER` | (none) | OIDC Provider Issuer URL. |
| `OIDC_CLIENT_ID` | (none) | OIDC Client ID. |
| `OIDC_CLIENT_SECRET` | (none) | OIDC Client Secret. |
| `OIDC_REDIRECT_URL` | (none) | OIDC Redirect Callback URL. |
| `OIDC_ADMIN_CLAIM` | `groups` | Claim to check for admin status. |
| `OIDC_ADMIN_VALUE` | `admin` | Value in the admin claim that grants admin rights. |
| `JWT_SECRET` | (none) | Secret used to sign OIDC session cookies. |

---

## Technical Architecture

### 1. Request Lifecycle & Routing
Requests flow from clients through the Balancer to the Agents:
- **Prioritization:** Requests enter a min-heap priority queue; higher integers = higher priority.
- **Scoring:** Agents are scored based on reputation, model residency (VRAM vs. Disk), and hardware load (CPU/Mem/VRAM).
- **Hedged Requests:** If enabled, the Balancer can send a second "hedged" request after a P90 timeout to minimize latency tails.

### 2. Virtual Models & Pipelines
Virtual models (defined in `CONFIG_PATH`) provide advanced routing:
- **Metric-based:** Routes to the `fastest`, `cheapest`, or `most_reliable` backing model.
- **Pipelines:** Executes a sequential chain of models where the output of one step becomes the input for the next.

### 3. Agent Edge Guardrails
Agents autonomously protect themselves via `checkCapabilities`:
- **Load Shedding:** Rejects requests if CPU or Memory thresholds are exceeded.
- **Performance Blocking:** Locally disables models with low TPS or high error rates.
- **Maintenance Loop:** Periodically "pre-warms" models designated as **Persistent** by the Balancer.

---

## Development Workflow

### Key Project Patterns
- **State Management:** The Balancer uses an `Actor` pattern (`pkg/balancer/state.go`) for thread-safe state snapshots.
- **Reverse Proxying:** The Agent uses `httputil.ReverseProxy` for robust communication with Ollama.
- **Frontend Polling:** The React app polls `/api/v1/status` every 5 seconds via `ClusterContext.tsx`.

### Testing
- **Integration Tests:** Located in `pkg/integration_test.go`. Verifies the full end-to-end request flow.
- **Backend Tests:** Run with `go test ./pkg/...`.
- **Frontend Linting:** Run `npm run lint` in the `frontend/` directory.

---

## API Integration
- **Ollama Compatible:** `/api/chat`, `/api/generate`, `/api/embeddings`, `/api/tags`.
- **OpenAI Compatible:** `/v1/chat/completions`, `/v1/completions`.
- **Management:** `/api/v1/status`, `/api/v1/nodes`, `/api/v1/keys`, `/api/v1/logs`.
