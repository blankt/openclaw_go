package runstate

import (
	"context"
	"time"
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

type Run struct {
	RunID     string    `json:"run_id"`
	Goal      string    `json:"goal"`
	Status    Status    `json:"status"`
	Final     string    `json:"final,omitempty"`
	Steps     int       `json:"steps,omitempty"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Store interface {
	Put(ctx context.Context, run Run) error
	Get(ctx context.Context, runID string) (Run, bool, error)
}
