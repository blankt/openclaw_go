package agent

import (
	"context"
	"io"
	"log"
	"testing"
	"time"

	ctxpack "openclaw_go/internal/context"
	"openclaw_go/internal/llm"
	"openclaw_go/internal/memory"
	"openclaw_go/internal/obs"
	"openclaw_go/internal/runtime"
	"openclaw_go/internal/tool"
)

func TestOrchestratorRunCompletes(t *testing.T) {
	registry := tool.NewRegistry()
	if err := registry.Register(tool.NewEchoTool()); err != nil {
		t.Fatalf("register echo: %v", err)
	}

	o := NewOrchestrator(Dependencies{
		LLM: llm.NewRetryingClient(llm.NewDeterministicClient(), llm.Policy{
			MaxPromptTokens: 1024,
			MaxRetries:      1,
			BaseBackoff:     5 * time.Millisecond,
		}),
		Packer: ctxpack.NewPacker(ctxpack.Config{
			MaxPromptTokens:   1024,
			ReserveForOutput:  200,
			MinMessagesToKeep: 2,
		}),
		Executor: runtime.NewExecutor(registry, runtime.Config{DefaultTimeout: time.Second}),
		Memory:   memory.NewInMemoryStore(),
		Metrics:  obs.NewMetrics(),
		Logger:   log.New(io.Discard, "", 0),
	})

	res, err := o.Run(context.Background(), Input{
		RunID:    "test-run",
		Goal:     "echo this",
		MaxSteps: 3,
	})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if res.Status != "completed" {
		t.Fatalf("expected completed status, got %q", res.Status)
	}
	if res.Steps < 1 {
		t.Fatalf("expected at least one step")
	}
}
