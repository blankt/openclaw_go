package app

import (
	"context"
	"fmt"
	"log"
	"time"

	"openclaw_go/internal/agent"
	ctxpack "openclaw_go/internal/context"
	"openclaw_go/internal/llm"
	"openclaw_go/internal/memory"
	"openclaw_go/internal/obs"
	"openclaw_go/internal/runtime"
	"openclaw_go/internal/tool"
)

// Runtime groups wired dependencies used by API and local demo flows.
type Runtime struct {
	Orchestrator *agent.Orchestrator
	Metrics      *obs.Metrics
}

// NewRuntime builds the minimal runnable OpenClaw dependency graph.
func NewRuntime(logger *log.Logger) (*Runtime, error) {
	registry := tool.NewRegistry()
	if err := registry.Register(tool.NewEchoTool()); err != nil {
		return nil, fmt.Errorf("register tool: %w", err)
	}

	metrics := obs.NewMetrics()
	store := memory.NewInMemoryStore()
	packer := ctxpack.NewPacker(ctxpack.Config{
		MaxPromptTokens:   800,
		ReserveForOutput:  200,
		MinMessagesToKeep: 2,
	})

	baseModel := llm.NewDeterministicClient()
	model := llm.NewRetryingClient(baseModel, llm.Policy{
		MaxPromptTokens: 800,
		MaxRetries:      2,
		BaseBackoff:     50 * time.Millisecond,
	})

	executor := runtime.NewExecutor(registry, runtime.Config{
		DefaultTimeout: 1500 * time.Millisecond,
	})

	orchestrator := agent.NewOrchestrator(agent.Dependencies{
		LLM:      model,
		Packer:   packer,
		Executor: executor,
		Memory:   store,
		Metrics:  metrics,
		Logger:   logger,
	})

	return &Runtime{Orchestrator: orchestrator, Metrics: metrics}, nil
}

// Run executes one local deterministic run for quick manual verification.
func Run(logger *log.Logger) error {
	rt, err := NewRuntime(logger)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := rt.Orchestrator.Run(ctx, agent.Input{
		RunID:    "demo-run",
		Goal:     "Say hello from OpenClaw Go migration loop",
		MaxSteps: 4,
	})
	if err != nil {
		return err
	}

	logger.Printf("status=%s steps=%d final=%q", result.Status, result.Steps, result.Final)
	logger.Printf("metrics=%v", rt.Metrics.Snapshot())
	return nil
}
