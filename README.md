# openclaw_go

Minimal, runnable bootstrap for migrating OpenClaw core capabilities from TypeScript to Go.

## What is implemented

- Plan-Act-Eval-Correct agent loop (`internal/agent`).
- Unified tool protocol + registry (`internal/tool`).
- Deterministic offline model adapter with retry and prompt budget policy (`internal/llm`).
- Context compression by token budget (`internal/context`).
- Runtime executor with idempotency cache, timeout, and compensation hook (`internal/runtime`).
- In-memory memory store (`internal/memory`).
- In-memory counters for observability (`internal/obs`).

## Repository layout

- `cmd/agentd/main.go`: runnable daemon entrypoint.
- `internal/app/bootstrap.go`: dependency wiring.
- `internal/agent/orchestrator.go`: main run loop.
- `docs/migration-matrix.md`: TS-to-Go semantic mapping.

## Quick start

```bash
go run ./cmd/agentd
```

## Verify

```bash
go test ./...
```

## Next milestones

1. Add browser execution adapters and tool sandboxing.
2. Add worker pool and queue-backed scheduling.
3. Replace in-memory memory/metrics with durable backends.
4. Integrate real LLM providers behind `llm.Client`.

