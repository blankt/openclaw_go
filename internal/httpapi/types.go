package httpapi

import (
	"context"
	"time"

	"openclaw_go/internal/agent"
	"openclaw_go/internal/runstate"
)

// Runner abstracts the orchestrator for HTTP handlers.
type Runner interface {
	Run(ctx context.Context, in agent.Input) (agent.Result, error)
}

type RunStateStore interface {
	Put(ctx context.Context, run runstate.Run) error
	Get(ctx context.Context, runID string) (runstate.Run, bool, error)
}

type createRunRequest struct {
	RunID    string `json:"run_id,omitempty"`
	Goal     string `json:"goal"`
	MaxSteps int    `json:"max_steps,omitempty"`
}

type runResponse struct {
	RunID     string    `json:"run_id"`
	Goal      string    `json:"goal,omitempty"`
	Status    string    `json:"status"`
	Final     string    `json:"final,omitempty"`
	Steps     int       `json:"steps,omitempty"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type errorResponse struct {
	Error string `json:"error"`
}
