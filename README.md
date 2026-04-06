# FlakyOllama
An intelligent balancer for Ollama, designed to be deployed on flaky nodes.

## Features
- **Intelligent Balancing**: Automatically distributes workloads across available nodes
- **Fault Tolerance**: Handles node failures gracefully, ensuring minimal disruption
- **Scalability**: Easily scales with your infrastructure, accommodating growth seamlessly
- **Model Balancing**: Distributes models about capable nodes
- **Capability Balancing**: Prioritizes nodes based on their capabilities, ensuring optimal performance

### Fault Tolerance
FlakyOllama is designed to handle node failures gracefully.
Every node is monitored externally at 10hz. Every request is cached and if a node fails, the request is retried on another node. This ensures minimal disruption to your services.

## Inference Allocation
FlakyOllama allocates inference requests based on the capabilities of the nodes.
Once a model is loaded on a node it is kept alive for a certain amount of time.
The same model is only sheduled on another node if a request sits stale for a certain amount of time.

When a model is running on a node, its performance is monitored and saved to a database. This allows FlakyOllama to make informed decisions about which nodes to prioritize for certain models, ensuring optimal performance.

## Capability Monitoring
FlakyOllama continuously monitors the capabilities of each node, including CPU, GPU, and memory.
Additionally it tracks the performance of each model on each node, allowing it to make informed decisions about which nodes to prioritize for certain models. This ensures that workloads are distributed efficiently, maximizing performance and minimizing latency.

Each node can be marked with resource limits and capabilities, such as GPU type, memory size, and CPU cores. FlakyOllama uses this information to prioritize nodes for certain workloads, ensuring that the most capable nodes are utilized for demanding tasks.

This allows for FlakyOllama to learn which nodes are best suited for certain workloads, further optimizing performance over time.

## Model Balancing
FlakyOllama distributes models files across capable or possible nodes. (e. g. a model that requires a GPU will only be distributed to nodes with a GPU)
This ensures that models are only loaded on nodes that can run them efficiently, maximizing performance and minimizing

## Agent and Balancer
FlakyOllama consists of two main components: the Agent and the Balancer.
- **Agent**: The Agent runs on each node and is a middleman between the node and the Balancer. It monitors the node's capabilities and performance, and communicates this information to the Balancer.
- **Balancer**: The Balancer is responsible for distributing workloads across the nodes based on the information received from the Agents. The balancer never directly communicates with the nodes, it only communicates with the Agents. This separation of concerns allows for a more modular and scalable architecture.

_Note: Both are available a single docker image, with an environment variable to specify the role. Additionally there is FlakyOllama-integrated image, which wraps the Agent to the official Ollama image, allowing for easy deployment on nodes._