package httpapi

import (
	"context"

	"openclaw_go/internal/agent"
)

// Runner abstracts the orchestrator for HTTP handlers.
type Runner interface {
	Run(ctx context.Context, in agent.Input) (agent.Result, error)
}

type createRunRequest struct {
	RunID    string `json:"run_id,omitempty"`
	Goal     string `json:"goal"`
	MaxSteps int    `json:"max_steps,omitempty"`
}

type createRunResponse struct {
	RunID  string `json:"run_id"`
	Status string `json:"status"`
	Final  string `json:"final"`
	Steps  int    `json:"steps"`
}

type errorResponse struct {
	Error string `json:"error"`
}
