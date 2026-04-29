# FlakyOllama Agent

The `agent` package provides the core functionality for running a FlakyOllama agent. It acts as a local telemetry shipper, task manager, and reverse proxy for a local Ollama instance, allowing it to connect to and be managed by a central FlakyOllama balancer cluster.

## Key Responsibilities

1. **Telemetry & Monitoring**: Regularly checks local hardware (CPU/VRAM usage) and active models, and reports this telemetry back to the balancer.
2. **Reverse Proxy**: Proxies requests (like `/v1/`, `/inference`, `/chat`, `/embeddings`) transparently to the underlying Ollama daemon.
3. **Task Management**: Accepts asynchronous model management commands from the balancer (e.g., `pull`, `create`, `push`, `copy`, `delete`) and executes them against the local Ollama instance.
4. **Log Shipping**: Buffers logs to a local disk queue (SQLite) and periodically ships them to the centralized log collector.
5. **Pre-warming**: Keeps designated persistent models warm by periodically checking if they are loaded and loading them if they are not.

## Environment Variables

The agent can be configured using the following environment variables:

| Variable | Description | Default |
| :--- | :--- | :--- |
| `AGENT_KEY` or `AGENT_TOKEN` | The authentication token used to register with the balancer. Overrides the `RemoteToken` in the configuration. | *None* |
| `AGENT_TIER` | The performance or billing tier of the agent. Used by the balancer for routing logic (e.g., prioritizing "dedicated" over "spot"). | `dedicated` |
| `AGENT_ADDRESS` | Overrides the address reported to the balancer during registration. Useful if the agent is running behind a NAT, Docker, or external load balancer, and needs to advertise a specific external address. | Discovered Hostname/IP |
| `AGENT_AUTH_TOKEN` | The authentication token required to access the agent's endpoints (`/telemetry`, `/tasks`, etc). If not set, it falls back to the agent's registration key (`AGENT_KEY`). Note: The agent also implicitly accepts the cluster's `RemoteToken` to ensure balancer connectivity. | *None* |
| `AGENT_AUTH_TOKEN_DISABLE` | Set to `true` to disable authentication for incoming requests to the agent. | `false` |

## Configuration (Config struct)

In addition to environment variables, the agent accepts a structured configuration (`config.Config`) that governs:
- **TLS**: Paths to `CertFile` and `KeyFile` if HTTPS is enabled, and `InsecureSkipVerify` for internal calls.
- **Hardware Allocations**: `MaxVRAMAllocated` and `MaxCPUAllocated` to cap resources reported to the balancer.
- **Tokens**: The `RemoteToken` used to authenticate with the balancer (and accepted for incoming cluster requests).
