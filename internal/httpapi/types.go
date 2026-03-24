package httpapi

import (
	"context"
	"strings"
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

type metricsResponse struct {
	Counters      map[string]int64 `json:"counters"`
	QueueDepth    int              `json:"queue_depth"`
	QueueCapacity int              `json:"queue_capacity"`
	WorkerCount   int              `json:"worker_count"`
}

type Config struct {
	QueueDepth            int
	RunTimeout            time.Duration
	WorkerCount           int
	IngressAPIKey         string
	CreateRunRPM          int
	CreateRunMaxBodyBytes int64
}

func DefaultConfig() Config {
	return Config{
		QueueDepth:            128,
		RunTimeout:            30 * time.Second,
		WorkerCount:           1,
		CreateRunRPM:          60,
		CreateRunMaxBodyBytes: 16 * 1024,
	}
}

func (c Config) normalize() Config {
	if c.QueueDepth <= 0 {
		c.QueueDepth = 128
	}
	if c.RunTimeout <= 0 {
		c.RunTimeout = 30 * time.Second
	}
	if c.WorkerCount <= 0 {
		c.WorkerCount = 1
	}
	if c.CreateRunRPM < 0 {
		c.CreateRunRPM = 0
	}
	if c.CreateRunMaxBodyBytes <= 0 {
		c.CreateRunMaxBodyBytes = 16 * 1024
	}
	c.IngressAPIKey = strings.TrimSpace(c.IngressAPIKey)
	return c
}
