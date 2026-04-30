# FlakyOllama Agent

The `agent` package provides the core functionality for running a FlakyOllama agent. It acts as a local telemetry shipper, task manager, and reverse proxy for a local Ollama instance, allowing it to connect to and be managed by a central FlakyOllama balancer cluster.

## Key Responsibilities

1. **Telemetry & Monitoring**: Regularly checks local hardware (CPU/VRAM usage) and active models, and reports this telemetry back to the balancer.
2. **Reverse Proxy**: Proxies requests (like `/v1/`, `/inference`, `/chat`, `/embeddings`) transparently to the underlying Ollama daemon.
3. **Local Capability Management**: Enforces local policies for model allowance, priorities, and health-based request rejection (model fitting).
4. **Task Management**: Accepts asynchronous model management commands from the balancer (e.g., `pull`, `create`, `push`, `copy`, `delete`) and executes them against the local Ollama instance.
5. **Log Shipping**: Buffers logs to a local disk queue (SQLite) and periodically ships them to the centralized log collector.
6. **Pre-warming**: Keeps designated persistent models warm by periodically checking if they are loaded and loading them if they are not.

## Environment Variables

The agent can be configured using the following environment variables:

| Variable | Description | Default |
| :--- | :--- | :--- |
| `AGENT_KEY` or `AGENT_TOKEN` | The authentication token used to register with the balancer. Overrides the `RemoteToken` in the configuration. | *None* |
| `AGENT_TIER` | The performance or billing tier of the agent. Used by the balancer for routing logic (e.g., prioritizing "dedicated" over "spot"). | `dedicated` |
| `AGENT_ADDRESS` | Overrides the address reported to the balancer during registration. Useful if the agent is running behind a NAT, Docker, or external load balancer, and needs to advertise a specific external address. | Discovered Hostname/IP |
| `AGENT_AUTH_TOKEN` | The authentication token required to access the agent's endpoints (`/telemetry`, `/tasks`, `/capabilities`, etc). | *None* |
| `AGENT_AUTH_TOKEN_DISABLE` | Set to `true` to disable authentication for incoming requests to the agent. | `false` |

### Capability Management Variables

| Variable | Description | Default |
| :--- | :--- | :--- |
| `AGENT_ALLOWED_MODELS` | Comma-separated list of allowed models. If empty, all are allowed. | *None* |
| `AGENT_DENY_MODELS` | Comma-separated list of models to explicitly block. | *None* |
| `AGENT_MODEL_PRIORITIES` | Comma-separated `model=priority` pairs (e.g., `llama3=100,phi3=10`). | *None* |
| `AGENT_REJECT_ON_HIGH_LOAD` | Set to `true` to enable rejection when hardware thresholds are exceeded. | `false` |
| `AGENT_MAX_CPU_THRESHOLD` | CPU usage percentage threshold for rejection. | `90.0` |
| `AGENT_MAX_MEM_THRESHOLD` | Memory usage percentage threshold for rejection. | `90.0` |
| `AGENT_MIN_PRIORITY_UNDER_LOAD`| Minimum model priority required to bypass load-based rejection. | `0` |
| `AGENT_MAX_ERROR_RATE` | Maximum local error rate (0.0-1.0) before a model is marked unhealthy. | `0.0` (disabled) |
| `AGENT_MAX_P95_LATENCY` | Maximum P95 latency in milliseconds before a model is marked unhealthy. | `0.0` (disabled) |
| `AGENT_MIN_TPS` | Minimum tokens-per-second throughput before a model is marked unhealthy. | `0.0` (disabled) |

## Local Capability Management

The Agent maintains a local policy to protect itself and ensure quality of service. It can reject requests before they hit the Ollama daemon based on:

- **Access Control**: Explicitly allowing or denying specific models.
- **Model Health (Fitting)**: Automatically blocking models that are performing poorly locally (high error rate, high latency, or low TPS).
- **Load Shedding**: Rejecting requests when CPU or Memory usage is too high. High-priority models can be configured to bypass this rejection via `AGENT_MIN_PRIORITY_UNDER_LOAD`.

### Policy API

You can view or update the current policy at runtime using the `/capabilities` endpoint:

- **GET `/capabilities`**: Returns the current policy JSON.
- **POST `/capabilities`**: Updates the policy. Expects a JSON body matching the `Policy` struct.

## Configuration (Config struct)

In addition to environment variables, the agent accepts a structured configuration (`config.Config`) that governs:
- **TLS**: Paths to `CertFile` and `KeyFile` if HTTPS is enabled, and `InsecureSkipVerify` for internal calls.
- **Hardware Allocations**: `MaxVRAMAllocated` and `MaxCPUAllocated` to cap resources reported to the balancer.
- **Tokens**: The `RemoteToken` used to authenticate with the balancer (and accepted for incoming cluster requests).
