package tool

import (
	"context"
	"encoding/json"
)

// Call is the protocol envelope for every tool invocation.
type Call struct {
	Name           string          `json:"name"`
	Input          json.RawMessage `json:"input"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
}

// Error describes a structured tool failure.
type Error struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

// Result is the protocol envelope for every tool response.
type Result struct {
	Name    string          `json:"name"`
	Success bool            `json:"success"`
	Output  json.RawMessage `json:"output,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// Tool is the execution contract used by orchestrator runtime.
type Tool interface {
	Name() string
	Execute(ctx context.Context, call Call) Result
}
