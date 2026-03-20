# OpenClaw Go Architecture (Current Phase)

This page visualizes the current migration phase after introducing run lifecycle tracking.

## Module Relationship Diagram

```mermaid
flowchart LR
    A[cmd/agentd] --> B[internal/httpapi]
    B --> C[internal/agent]
    B --> D[internal/runstate]
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
    participant ORC as agent.Orchestrator
    participant LLM as llm.Client
    participant EX as runtime.Executor
    participant TOOL as tool.Registry

    Client->>API: POST /v1/runs {goal, run_id?}
    API->>RS: Put(status=queued)
    API->>RS: Put(status=running)
    API->>ORC: Run(input)
    ORC->>LLM: Decide(messages)
    ORC->>EX: Execute(action)
    EX->>TOOL: Call(tool)
    TOOL-->>EX: Result
    EX-->>ORC: ToolResult
    ORC-->>API: Result(status/final/steps)
    API->>RS: Put(status=completed|failed)
    API-->>Client: 200 JSON run state

    Client->>API: GET /v1/runs/{id}
    API->>RS: Get(id)
    RS-->>API: Run state
    API-->>Client: 200 JSON run state / 404
```

## Notes

- This phase keeps execution synchronous for easier migration verification.
- Run state is currently in-memory and scoped to a single process.
- Next phase can switch `POST /v1/runs` to async queue + worker without breaking `GET /v1/runs/{id}` contract.

