# FlakyOllama
An intelligent load balancer for Ollama, built specifically for unreliable and intermittent nodes.

## Features
- **Smart Load Balancing**: Automatically distributes inference workloads across active nodes for optimal throughput.
- **Robust Fault Tolerance**: Seamlessly handles node failures by failing over requests, ensuring steady availability.
- **Seamless Scalability**: Easily expand your infrastructure as your needs grow.
- **Dynamic Model Allocation**: Intelligently assigns models only to nodes capable of running them.
- **Capability-Aware Routing**: Prioritizes nodes based on hardware capabilities and historical performance to maximize efficiency.

### Fault Tolerance
FlakyOllama provides robust resilience against sudden node dropouts.
Every node is externally monitored at 10Hz. All incoming requests are safely queued, and if a node fails mid-inference, the request is transparently retried on another available node. This ensures zero downtime for your applications.

## Inference Allocation
FlakyOllama intelligently schedules inference requests based on hardware capabilities and model state.
Once loaded, a model is kept alive in memory for a configurable duration. To prevent unnecessary duplication, a model is only scheduled on an additional node if pending requests queue up past a stale threshold.

As models run, their performance metrics are recorded in a database. FlakyOllama leverages these metrics to learn which nodes handle specific models best, continually refining its routing decisions for optimal latency.

## Capability Monitoring
FlakyOllama continuously tracks node health and resources, including CPU, GPU availability, and memory usage.
Nodes can be configured with specific resource constraints and capabilities (such as GPU architecture, VRAM size, and CPU core count). FlakyOllama aligns these capabilities with model requirements to ensure demanding workloads are routed exactly where they belong.

By combining real-time resource tracking and historical model performance, the balancer continuously adapts, reducing latency and maximizing overall system efficiency over time.

## Model Distribution
FlakyOllama distributes model files only to nodes that can actually run them. For example, a model requiring significant GPU acceleration will exclusively be sent to GPU-equipped nodes. This guarantees that model weights are loaded efficiently, preventing resource exhaustion on underpowered hardware.

## Architecture: Agent & Balancer
The system is built upon two distinct components: the Agent and the Balancer.
- **Agent**: Deployed on each node, the Agent acts as a bridge between the local Ollama instance and the Balancer. It monitors local hardware capabilities and performance, relaying telemetry back to the central Balancer.
- **Balancer**: The brain of the operation. It aggregates telemetry from all Agents and orchestrates workload distribution. The Balancer communicates strictly with Agents—never directly with the Ollama nodes—ensuring a clean, secure, and scalable architecture.

_Note: Both components are packaged into a single Docker image, toggleable via an environment variable._

---

## Getting Started

### Running from Source
FlakyOllama requires Go 1.21+.

```bash
# Start the balancer
ROLE=balancer BALANCER_ADDR=0.0.0.0:8080 go run main.go

# Start an agent (in a separate terminal)
ROLE=agent AGENT_ID=node-1 BALANCER_URL=http://localhost:8080 OLLAMA_URL=http://localhost:11434 go run main.go
```

### Docker Compose
A typical deployment uses Docker Compose to run a central Balancer and multiple Agents.

```yaml
version: '3.8'
services:
  balancer:
    image: flakyollama
    environment:
      - ROLE=balancer
    ports:
      - "8080:8080"
  
  agent-1:
    image: flakyollama
    environment:
      - ROLE=agent
      - AGENT_ID=agent-1
      - BALANCER_URL=http://balancer:8080
      - OLLAMA_URL=http://host.docker.internal:11434
```

## Configuration

FlakyOllama components are configured primarily via environment variables. The Balancer can also load advanced routing weights and thresholds from a JSON file.

### Environment Variables

**Global**
* `ROLE`: The component to run. Valid values: `balancer`, `agent`. Default is `balancer`.
* `CONFIG_PATH`: Path to the JSON configuration file for the Balancer.

**Balancer Variables**
* `BALANCER_ADDR`: Host and port for the Balancer to listen on. Default: `0.0.0.0:8080`.
* `DB_PATH`: Path to the SQLite database file for metrics and learned constraints. Default: `flakyollama.db`.
* `BALANCER_TOKEN`: (Optional) A Bearer token required for clients to access inference APIs.

**Agent Variables**
* `AGENT_ID`: A unique identifier for the agent node. Default: `agent-1`.
* `AGENT_ADDR`: Host and port for the Agent to listen on. Default: `0.0.0.0:8081`.
* `BALANCER_URL`: The URL of the Balancer. Default: `http://localhost:8080`.
* `OLLAMA_URL`: The URL of the local Ollama instance. Default: `http://localhost:11434`.
* `AGENT_TOKEN`: (Optional) A Bearer token used by the Balancer to securely communicate with the Agent.

### Balancer JSON Configuration (`config.json`)
If `CONFIG_PATH` is set, the Balancer loads these settings. If omitted, defaults are used.

```json
{
  "keep_alive_duration_sec": 300,
  "stale_threshold": 5,
  "load_threshold": 80.0,
  "poll_interval_ms": 100,
  "weights": {
    "cpu_load_weight": 1.0,
    "latency_weight": 1.0,
    "success_rate_weight": 1.0,
    "loaded_model_bonus": 2.0
  },
  "circuit_breaker": {
    "error_threshold": 3,
    "cooloff_sec": 60
  },
  "stall_timeout_sec": 15,
  "hedging_percentile": 0.95
}
```

## API Reference

The Balancer acts as a drop-in replacement for the standard Ollama API and provides an OpenAI-compatible layer.

### Ollama Compatibility
* `POST /api/generate` - Generate a completion.
* `POST /api/chat` - Generate a chat completion.
* `POST /api/show` - Show model information.
* `GET /api/tags` - List all loaded models across the cluster.

### OpenAI Compatibility
* `POST /v1/chat/completions` - Chat completions.
* `POST /v1/completions` - Text completions.
* `GET /v1/models` - List active models.

### Cluster Management
* `GET /status` - Internal health and routing status.
* `GET /metrics` - Prometheus metrics (e.g., node health, latency, queue depth).
* `POST /api/manage/node/drain?id=<agent-id>` - Mark a node as draining to prevent new workloads.
* `POST /api/manage/node/undrain?id=<agent-id>` - Remove draining status.

## Example Usage

Send a standard Ollama request to the Balancer:

```bash
curl http://localhost:8080/api/generate -d '{
  "model": "llama2",
  "prompt": "Why is the sky blue?",
  "stream": false
}'
```

Send an OpenAI-compatible request:

```bash
curl http://localhost:8080/v1/chat/completions -H "Content-Type: application/json" -d '{
  "model": "llama2",
  "messages": [{"role": "user", "content": "Hello!"}]
}'
```