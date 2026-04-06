# FlakyOllama

FlakyOllama is a Go project intended to provide a resilient, capability-aware load balancer for Ollama nodes.

> Current repository state: early scaffold/WIP.

## Requirements

- Go 1.26+

## Build

From the repository root:

```bash
go build ./...
```

To build a single local binary:

```bash
go build -o FlakyOllama .
```

## Test

```bash
go test ./...
```

## Run

```bash
go run .
```

## Project Status

This repository is currently minimal and does not yet implement the balancer/agent runtime described in the original concept. The current entrypoint is `main.go`.

## Roadmap (high-level)

- Add balancer service and request routing
- Add node agent and heartbeat/capability reporting
- Add retry/failover logic for flaky workers
- Add integration tests and deployment artifacts
