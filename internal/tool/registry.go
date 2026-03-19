package tool

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Registry provides concurrent-safe tool lookup and dispatch.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

func (r *Registry) Register(t Tool) error {
	if t == nil {
		return fmt.Errorf("tool is nil")
	}
	name := t.Name()
	if name == "" {
		return fmt.Errorf("tool name is empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool %q already registered", name)
	}
	r.tools[name] = t
	return nil
}

func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (r *Registry) Call(ctx context.Context, call Call) Result {
	t, ok := r.Get(call.Name)
	if !ok {
		return Result{
			Name:    call.Name,
			Success: false,
			Error: &Error{
				Code:      "tool_not_found",
				Message:   fmt.Sprintf("tool %q is not registered", call.Name),
				Retryable: false,
			},
		}
	}
	return t.Execute(ctx, call)
}
