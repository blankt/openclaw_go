# OpenClaw Go Architecture (Current Phase)

This page visualizes the current migration phase after introducing async run execution.

## Module Relationship Diagram

```mermaid
flowchart LR
    A[cmd/agentd] --> B[internal/httpapi]
    B --> D[internal/runstate]
    B --> Q[job queue in Server]
    Q --> W[multi worker goroutines]
    W --> C[internal/agent]
    C --> E[internal/context]
    C --> F[internal/llm]
    C --> G[internal/runtime]
    C --> H[internal/memory]
    C --> I[internal/obs]
    G --> J[internal/tool]
```

## Data Flow Diagram

```mermaid
sequenceDiagram
    participant Client
    participant API as httpapi.Server
    participant RS as runstate.Store
    participant Q as jobs chan
    participant W as worker pool
    participant ORC as agent.Orchestrator
    participant LLM as llm.Client
    participant EX as runtime.Executor
    participant TOOL as tool.Registry

    Client->>API: POST /v1/runs {goal, run_id?}
    API->>RS: Put(status=queued)
    API->>Q: enqueue job
    API-->>Client: 202 Accepted + queued state

    W->>Q: workers receive jobs
    W->>RS: Put(status=running)
    W->>ORC: Run(input)
    ORC->>LLM: Decide(messages)
    ORC->>EX: Execute(action)
    EX->>TOOL: Call(tool)
    TOOL-->>EX: Result
    EX-->>ORC: ToolResult
    ORC-->>W: Result(status/final/steps)
    W->>RS: Put(status=completed|failed)

    Client->>API: GET /v1/runs/{id}
    API->>RS: Get(id)
    RS-->>API: Run state
    API-->>Client: 200 JSON run state / 404
```

## Notes

- `POST /v1/runs` is asynchronous and returns quickly with queue-backed status.
- `POST /v1/runs` can be guarded by optional Bearer ingress API key.
- `POST /v1/runs` supports per-client fixed-window rate limiting (`429` + `Retry-After`).
- `POST /v1/runs` enforces strict JSON decoding and max request body size guardrails (`413`/`400`).
- `httpapi.Config` controls queue depth, worker count, and per-run timeout.
- `httpapi.Server.Close` drains accepted jobs before worker shutdown.
- Queue dispatch remains FIFO with concurrent execution when multiple workers are configured.
- Run state is currently in-memory and scoped to a single process.
