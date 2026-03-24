# openclaw_go

Minimal, runnable bootstrap for migrating OpenClaw core capabilities from TypeScript to Go.

## What is implemented

- Plan-Act-Eval-Correct agent loop (`internal/agent`).
- Unified tool protocol + registry (`internal/tool`).
- Deterministic offline model adapter with retry and prompt budget policy (`internal/llm`).
- Context compression by token budget (`internal/context`).
- Runtime executor with idempotency cache, timeout, and compensation hook (`internal/runtime`).
- In-memory memory store (`internal/memory`).
- In-memory run lifecycle store (`internal/runstate`) with `queued/running/completed/failed`.
- In-memory counters for observability (`internal/obs`).
- ServeMux HTTP API (`internal/httpapi`) with:
  - `GET /healthz`
  - `POST /v1/runs` (async enqueue, returns `202 Accepted`)
  - `GET /v1/runs/{id}` (poll run status)
  - `GET /v1/metrics` (in-memory counters + scheduler metadata)
- Single worker graceful shutdown (`httpapi.Server.Close`) and configurable queue/timeout.
- Configurable worker parallelism (`AGENTD_WORKER_COUNT`) for concurrent run processing.

## Repository layout

- `cmd/agentd/main.go`: long-running HTTP daemon.
- `internal/app/bootstrap.go`: dependency wiring.
- `internal/agent/orchestrator.go`: main run loop.
- `internal/httpapi/server.go`: API handlers, queue, and single worker.
- `internal/runstate`: run lifecycle persistence contracts and memory store.
- `docs/architecture.md`: module relationship and data-flow diagrams.
- `docs/migration-matrix.md`: TS-to-Go semantic mapping.

## Quick start

```bash
go run ./cmd/agentd
```

By default, it listens on `:8080`.

Environment options:

- `AGENTD_ADDR` (default `:8080`)
- `AGENTD_QUEUE_DEPTH` (default `128`)
- `AGENTD_RUN_TIMEOUT` (default `30s`, Go duration format)
- `AGENTD_WORKER_COUNT` (default `1`)

## API smoke test

```bash
curl -sS http://127.0.0.1:8080/healthz
curl -sS -X POST http://127.0.0.1:8080/v1/runs \
  -H 'Content-Type: application/json' \
  -d '{"run_id":"demo-1","goal":"hello from api"}'
curl -sS http://127.0.0.1:8080/v1/runs/demo-1
curl -sS http://127.0.0.1:8080/v1/metrics
```

## Verify

```bash
go test ./...
```

## Next milestones

1. Add fair queue policies and per-tenant scheduling.
2. Replace in-memory memory/metrics with durable backends.
3. Integrate real LLM providers behind `llm.Client`.
4. Add security guardrails (auth, rate limit, tool policy).
