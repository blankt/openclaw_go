package runtime

import (
	"context"
	"sync"
	"time"

	"openclaw_go/internal/tool"
)

type Registry interface {
	Call(ctx context.Context, call tool.Call) tool.Result
}

type CompensationFunc func(ctx context.Context) error

type Action struct {
	Call         tool.Call
	Timeout      time.Duration
	Compensation CompensationFunc
}

type Config struct {
	DefaultTimeout time.Duration
}

type Executor struct {
	registry Registry
	cfg      Config

	mu      sync.Mutex
	results map[string]tool.Result
}

func NewExecutor(registry Registry, cfg Config) *Executor {
	if cfg.DefaultTimeout <= 0 {
		cfg.DefaultTimeout = 2 * time.Second
	}
	return &Executor{
		registry: registry,
		cfg:      cfg,
		results:  make(map[string]tool.Result),
	}
}

func (e *Executor) Execute(ctx context.Context, action Action) tool.Result {
	if action.Call.IdempotencyKey != "" {
		e.mu.Lock()
		cached, ok := e.results[action.Call.IdempotencyKey]
		e.mu.Unlock()
		if ok {
			return cached
		}
	}

	timeout := action.Timeout
	if timeout <= 0 {
		timeout = e.cfg.DefaultTimeout
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result := e.registry.Call(execCtx, action.Call)
	if !result.Success && action.Compensation != nil {
		_ = action.Compensation(execCtx)
	}

	if action.Call.IdempotencyKey != "" {
		e.mu.Lock()
		e.results[action.Call.IdempotencyKey] = result
		e.mu.Unlock()
	}
	return result
}
