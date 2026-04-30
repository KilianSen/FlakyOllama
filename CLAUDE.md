# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

### Go backend (run from project root)
```bash
go build ./...                              # Build all packages
go test ./...                               # Run all tests
go test ./pkg/balancer/ -run TestSelectAgent  # Run a single test
go run main.go                             # Run with ROLE=balancer (default)
ROLE=balancer go run main.go               # Run balancer
ROLE=agent go run main.go                  # Run agent
```

### Frontend (run from `frontend/`)
```bash
npm run dev     # Start Vite dev server
npm run build   # TypeScript compile + Vite bundle
npm run lint    # ESLint
```

### Docker development stack
```bash
docker compose -f docker-compose.dev.yml up
```
This starts balancer on `:8080`, frontend on `:3000`, and a dev agent on `:8081`, all using `dev-token` for auth.

## Architecture

FlakyOllama is a load balancer and orchestration layer for [Ollama](https://ollama.com). It exposes OpenAI-compatible and Ollama-native APIs and distributes inference requests across a fleet of Ollama nodes.

### Single binary, two roles

`main.go` switches between two modes via the `ROLE` environment variable:

- **`balancer`** — the central coordinator. Accepts client requests, manages the priority queue, selects agents, stores metrics in SQLite, and serves the management API.
- **`agent`** — a sidecar that runs alongside a local Ollama instance. It proxies inference requests, reports hardware telemetry, and ships logs back to the balancer.

### Balancer internals (`pkg/balancer/`)

| File | Responsibility |
|---|---|
| `balancer.go` | `Balancer` struct, route setup via chi, lifecycle |
| `state.go` | `Actor` — goroutine-safe cluster state with copy-on-read snapshot |
| `queue.go` | `RequestQueue` — min-heap priority queue; higher `Priority` int = dequeued first; ties broken by sequence number |
| `routing.go` | `SelectAgent` — scores each node (reputation, loaded model bonus, VRAM, CPU, workload) and picks the best |
| `hedging.go` | `DoHedgedRequest` — sends a speculative second request after a P90 timeout if hedging is enabled |
| `proxy.go` | Forwards requests to the chosen agent; `workloadBody` wrapper decrements the node workload counter on `Close()` |
| `pipeline.go` | Executes multi-step `VirtualModel` pipelines (chain of model calls) |
| `storage.go` | `SQLiteStorage` — SQLite/WAL persistence for metrics, logs, keys, users, policies, model requests |
| `jobs.go` | `JobManager` — tracks long-running async operations (model pulls etc.) |
| `stall.go` | Detects stalled/hanging requests and requeues them |
| `oidc.go` | OIDC login/callback handlers; issues JWT session cookies |
| `handlers_*.go` | HTTP handlers split by domain: OpenAI, Ollama native, management |

**Request flow:** client → `AuthMiddleware` → `HandleOpenAIChat` / `HandleChat` / `HandleGenerate` → `DoHedgedRequest` → `RequestQueue.Push` → `SelectAgent` → `sendToAgentWithContext`.

### Agent internals (`pkg/agent/`)

- `agent.go` — `Agent` struct; reverse-proxies inference paths directly to Ollama; handles telemetry, registration, log shipping, and the persistent-model maintenance loop.
- `monitoring/monitor.go` — polls CPU, memory, and VRAM (via go-nvml for NVIDIA GPUs).
- `ollama/client.go` — typed HTTP client for the local Ollama API.
- `tasks/manager.go` — tracks async tasks (pull, push, create) running on the agent.
- `logging/disk_queue.go` — SQLite-backed queue that buffers log entries and ships them to the balancer every 5 s.

### Protocol adapters (`pkg/protocols/`)

`Adapter` interface translates between external API formats and the internal Ollama format. `openai.go` implements the OpenAI Chat Completions ↔ Ollama translation (including SSE streaming).

### Shared packages (`pkg/shared/`)

- `config/config.go` — `Config` struct, JSON file loading, defaults. All routing weights, circuit-breaker settings, OIDC config, virtual-model definitions live here.
- `models/models.go` — all cross-cutting data types (`NodeStatus`, `ClusterStatus`, `VirtualModelConfig`, etc.).
- `auth/auth.go` — Bearer-token middleware used by both balancer and agent.
- `metrics/ema.go` — exponential moving average; `prometheus.go` — Prometheus registry.
- `logging/logger.go` — structured global logger with a `LogSink` interface (implemented by both `Balancer` and `Agent`).

### Virtual models

Configured in `Config.VirtualModels`, three types are supported:
- **`metric`** — routes to the backing model with the best metric (`fastest`, `most_reliable`, `cheapest`).
- **`pipeline`** — chains multiple model calls sequentially, passing output as input to the next step.
- **`arena`** — (declared in types, routing not yet fully wired).

### Frontend (`frontend/`)

React 19 + Vite + TailwindCSS v4 + shadcn/ui. All API calls go through `src/api.ts` which uses relative paths (proxied by Vite dev server / Nginx in production). Auth token is stored in `localStorage` under `BALANCER_TOKEN` or injected via `VITE_BALANCER_TOKEN`. The `ClusterContext` polls `/api/v1/status` and provides cluster state to the whole app. Pages live in `src/pages/`, each calling the typed SDK in `api.ts`.

### Authentication

Two-layer system:
1. **API keys** — Bearer tokens for clients (`BALANCER_TOKEN`) and agents (`AGENT_TOKEN`). Keys are stored in SQLite with status (`pending`/`active`/`rejected`) and optional quota/credit limits.
2. **OIDC** — optional SSO via `OIDC_*` env vars; admin status is derived from a configurable claim/value pair. Session is a signed JWT cookie using `JWT_SECRET`.

### Key environment variables

| Variable | Default | Purpose |
|---|---|---|
| `ROLE` | `balancer` | Switch binary mode |
| `BALANCER_TOKEN` | — | Token clients present to the balancer |
| `AGENT_TOKEN` | — | Token agents use to register; also sent by balancer to agents |
| `DB_PATH` | `flakyollama.db` | SQLite file path (balancer) |
| `BALANCER_ADDR` | `0.0.0.0:8080` | Balancer listen address |
| `AGENT_ADDR` | `0.0.0.0:8081` | Agent listen address |
| `BALANCER_URL` | `http://localhost:8080` | Balancer URL seen by agents |
| `OLLAMA_URL` | `http://localhost:11434` | Ollama URL seen by agents |
| `AGENT_ID` | hostname | Node identifier |
| `AGENT_ADDRESS` | — | Override address advertised to balancer |
| `JWT_SECRET` | (insecure default) | Signs OIDC session cookies — must be changed |
| `CONFIG_PATH` | — | Path to JSON config file |
