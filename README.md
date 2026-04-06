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

_Note: Both components are packaged into a single Docker image, toggleable via an environment variable. Additionally, we provide the `FlakyOllama-integrated` image, which bundles the Agent directly alongside the official Ollama runtime for frictionless deployment on worker nodes._
