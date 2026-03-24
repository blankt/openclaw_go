# OpenClaw TS -> Go Migration Matrix

This matrix tracks semantic parity goals rather than line-by-line translation.

| TS capability | Go package/symbol | Phase | Semantic parity target | Notes |
| --- | --- | --- | --- | --- |
| Agent loop (plan/act/eval/correct) | `internal/agent.Orchestrator.Run` | M1 | Same loop behavior with bounded steps and correction path | Done (bootstrap) |
| Tool protocol | `internal/tool.Call`, `internal/tool.Result`, `internal/tool.Registry` | M1 | Standard request/response envelope and deterministic dispatch | Done (bootstrap) |
| Model adapter | `internal/llm.Client`, `internal/llm.RetryingClient` | M1 | Enforced prompt budget and retry policy hooks | Done (stub client) |
| Context compression | `internal/context.Packer.Pack` | M1 | Trim history to fit token budget while preserving recency | Done |
| Runtime execution safety | `internal/runtime.Executor.Execute` | M2 | Idempotency key cache, timeout controls, compensation hook | Done (single-process) |
| Memory | `internal/memory.Store`, `internal/memory.InMemoryStore` | M2 | Persist run history across loop steps | Done (in-memory) |
| Run lifecycle state | `internal/runstate.Store`, `internal/httpapi` | M2 | Expose queued/running/completed/failed lifecycle with query API | Done (in-memory + GET by id) |
| Async scheduling baseline | `internal/httpapi.Server` worker queue | M3 | Enqueue run request and process via worker pool | Done (202 + in-process queue) |
| Worker lifecycle controls | `internal/httpapi.Server.Close`, `httpapi.Config` | M3 | Graceful drain on shutdown and configurable queue depth/timeout/worker count | Done |
| Observability | `internal/obs.Metrics`, `internal/httpapi` | M2 | Emit and expose loop/tool/runtime counters for diagnostics | Done (in-memory counters + GET /v1/metrics) |
| Browser execution | `internal/browser/*` | M3 | Equivalent browser action semantics and retries | Planned |
| Concurrent scheduling | `internal/scheduler/*` | M3 | Multi-worker fairness and cancellation semantics | Planned |
| Security governance | `internal/httpapi.requireIngressAuth`, `internal/httpapi.requireCreateRunRateLimit`, `internal/httpapi.handleCreateRun` | M4 | API key, per-client rate limiting, strict JSON, and bounded request body on mutating endpoint | Done (Bearer auth + 429 + strict decode + 413 guardrail for POST /v1/runs) |

## Milestone acceptance

- M1: `go run ./cmd/agentd` serves health and run APIs.
- M2: Run lifecycle (`queued/running/completed/failed`) is persisted and queryable.
- M3: `POST /v1/runs` returns `202`, worker pool updates terminal state asynchronously, and shutdown drains in-flight jobs.
- M4: Ingress guardrails enforce API key policy and per-client rate limiting on run submission.
