package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	ctxpack "openclaw_go/internal/context"
	"openclaw_go/internal/llm"
	"openclaw_go/internal/memory"
	"openclaw_go/internal/obs"
	"openclaw_go/internal/runtime"
	"openclaw_go/internal/tool"
)

type Input struct {
	RunID    string
	Goal     string
	MaxSteps int
}

type Result struct {
	Status string
	Final  string
	Steps  int
}

type Dependencies struct {
	LLM      llm.Client
	Packer   *ctxpack.Packer
	Executor *runtime.Executor
	Memory   memory.Store
	Metrics  *obs.Metrics
	Logger   *log.Logger
}

type Orchestrator struct {
	deps Dependencies
}

func NewOrchestrator(deps Dependencies) *Orchestrator {
	return &Orchestrator{deps: deps}
}

func (o *Orchestrator) Run(ctx context.Context, in Input) (Result, error) {
	if in.MaxSteps <= 0 {
		in.MaxSteps = 4
	}
	if in.RunID == "" {
		in.RunID = "run-default"
	}
	if in.Goal == "" {
		return Result{}, fmt.Errorf("goal is required")
	}

	o.deps.Metrics.Inc("run.started")
	_ = o.deps.Memory.Append(ctx, in.RunID, memory.Entry{Role: string(llm.RoleUser), Content: in.Goal})

	for step := 1; step <= in.MaxSteps; step++ {
		o.deps.Metrics.Inc("loop.iteration")

		history, err := o.deps.Memory.List(ctx, in.RunID, 50)
		if err != nil {
			return Result{}, fmt.Errorf("load memory: %w", err)
		}

		messages := make([]llm.Message, 0, len(history)+1)
		messages = append(messages, llm.Message{Role: llm.RoleSystem, Content: "Run Plan-Act-Eval-Correct and emit deterministic decisions."})
		for _, e := range history {
			messages = append(messages, llm.Message{Role: llm.Role(e.Role), Content: e.Content})
		}

		packed := o.deps.Packer.Pack(messages)
		decision, usage, err := o.deps.LLM.Decide(ctx, llm.Request{Messages: packed, MaxOutputTokens: 256})
		if err != nil {
			o.deps.Metrics.Inc("llm.error")
			return Result{}, fmt.Errorf("llm decide: %w", err)
		}
		o.deps.Metrics.Add("tokens.prompt", int64(usage.PromptTokens))
		o.deps.Metrics.Add("tokens.completion", int64(usage.CompletionTokens))
		_ = o.deps.Memory.Append(ctx, in.RunID, memory.Entry{Role: string(llm.RoleAssistant), Content: decision.Reason})

		switch decision.Kind {
		case llm.DecisionDone:
			o.deps.Metrics.Inc("run.completed")
			_ = o.deps.Memory.Append(ctx, in.RunID, memory.Entry{Role: string(llm.RoleAssistant), Content: decision.Final})
			return Result{Status: "completed", Final: decision.Final, Steps: step}, nil
		case llm.DecisionTool:
			call := tool.Call{
				Name:           decision.ToolName,
				Input:          decision.ToolInput,
				IdempotencyKey: fmt.Sprintf("%s-%d", in.RunID, step),
			}
			res := o.deps.Executor.Execute(ctx, runtime.Action{Call: call})
			if !res.Success {
				o.deps.Metrics.Inc("tool.error")
				msg := "tool failed"
				if res.Error != nil {
					msg = res.Error.Message
				}
				_ = o.deps.Memory.Append(ctx, in.RunID, memory.Entry{Role: string(llm.RoleAssistant), Content: "Correction: " + msg})
				continue
			}
			o.deps.Metrics.Inc("tool.success")
			payload := string(res.Output)
			if !json.Valid(res.Output) {
				payload = fmt.Sprintf("%q", payload)
			}
			_ = o.deps.Memory.Append(ctx, in.RunID, memory.Entry{Role: string(llm.RoleTool), Content: payload})
			if o.deps.Logger != nil {
				o.deps.Logger.Printf("tool=%s output=%s", call.Name, payload)
			}
		default:
			o.deps.Metrics.Inc("llm.invalid_decision")
			_ = o.deps.Memory.Append(ctx, in.RunID, memory.Entry{Role: string(llm.RoleAssistant), Content: "Correction: unsupported decision kind"})
		}
	}

	o.deps.Metrics.Inc("run.max_steps")
	return Result{Status: "max_steps", Final: "run reached max steps", Steps: in.MaxSteps}, nil
}
