# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Build
go build ./...

# Run tests
go test ./...
go test -v ./pkg/balancer/
go test -v ./pkg/

# Run balancer
ROLE=balancer BALANCER_ADDR=0.0.0.0:8080 go run main.go

# Run agent
ROLE=agent AGENT_ID=node-1 BALANCER_URL=http://localhost:8080 OLLAMA_URL=http://localhost:11434 go run main.go

# Frontend
cd frontend && npm install && npm run dev
```

## Architecture

FlakyOllama is a load balancer for Ollama clusters with unreliable/intermittent nodes. A single `main.go` entry point branches into two roles via the `ROLE` env var.

### Components

**Balancer** (`pkg/balancer/`) — central orchestrator:
- `balancer.go`: Initialization, HTTP mux setup, chi router wiring
- `routing.go`: Agent selection scoring (model availability, latency history, CPU load, success rate, per-user policy weights)
- `state.go`: Concurrent-safe cluster state actor; all agent state mutations go through this
- `handlers_ollama.go` / `handlers_openai.go`: API surface (Ollama-compatible + OpenAI-compatible endpoints)
- `proxy.go`: Request forwarding to selected agent
- `hedging.go`: Speculative execution — fires duplicate requests after a configurable latency percentile and cancels the slower one
- `storage.go`: SQLite persistence for metrics, user policies, and token store
- `tasks.go`: Async job manager for model pull/push/delete across nodes
- `circuit_breaker.go`: Per-agent failure tracking with configurable threshold and cooloff
- `queue.go`: Request queuing when all agents are saturated
- `oidc.go`: OpenID Connect middleware

**Agent** (`pkg/agent/`) — runs on each Ollama node:
- `agent.go`: Core loop — polls balancer with telemetry, proxies inference requests to local Ollama, executes task commands from balancer, buffers logs to disk
- `monitoring/`: Hardware telemetry (CPU, GPU, VRAM) at 10 Hz via gopsutil + go-nvml
- `ollama/`: Thin client for local Ollama (pull, create, chat, generate)
- `tasks/`: Async task executor for model management operations
- `logging/`: SQLite-backed disk queue for log buffering

**Shared** (`pkg/shared/`):
- `models/`: Core data types (`NodeStatus`, `User`, `AgentTelemetry`, etc.) shared between balancer and agent
- `config/`: Config structs and JSON/env loading
- `auth/`: Bearer token + OIDC middleware
- `metrics/`: Prometheus metrics registration
- `protocols/`: OpenAI ↔ Ollama request/response translation

### Key Data Flow

1. Clients hit balancer at `/api/generate`, `/api/chat`, `/v1/chat/completions`
2. Balancer scores registered agents via `routing.go` (model loaded? latency? load?)
3. Request forwarded to winning agent's `/api/generate` (or hedged to top-N)
4. Agent proxies to local Ollama, streams response back
5. Agents periodically POST telemetry to balancer; balancer updates cluster state actor

### Configuration

Environment variables:

| Var | Component | Purpose |
|-----|-----------|---------|
| `ROLE` | both | `balancer` or `agent` |
| `BALANCER_ADDR` | balancer | Listen address (default `0.0.0.0:8080`) |
| `AGENT_ID` | agent | Node identifier |
| `BALANCER_URL` | agent | Balancer address to register with |
| `OLLAMA_URL` | agent | Local Ollama address |
| `DB_PATH` | balancer | SQLite database path |
| `CONFIG_PATH` | both | JSON config file |
| `BALANCER_TOKEN` | both | Bearer token for client→balancer auth |
| `AGENT_TOKEN` | both | Bearer token for agent→balancer auth |
| `OIDC_*` | balancer | OIDC provider config |

JSON config file key fields: `keep_alive_duration_sec`, `stale_threshold`, `load_threshold`, `poll_interval_ms`, `weights` (routing score weights), `circuit_breaker` (threshold + cooloff), `hedging_percentile`.

### Dependencies

- `github.com/go-chi/chi/v5` — HTTP router
- `github.com/mattn/go-sqlite3` — persistence (requires CGO)
- `github.com/prometheus/client_golang` — metrics
- `github.com/shirou/gopsutil/v3` — system monitoring
- `github.com/NVIDIA/go-nvml` — GPU monitoring
- `github.com/golang-jwt/jwt/v5` + `github.com/coreos/go-oidc/v3` — auth
