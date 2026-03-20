package runstate

import (
	"context"
	"sync"
	"time"
)

type InMemoryStore struct {
	mu   sync.RWMutex
	data map[string]Run
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{data: make(map[string]Run)}
}

func (s *InMemoryStore) Put(_ context.Context, run Run) error {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()

	prev, exists := s.data[run.RunID]
	if !exists {
		if run.CreatedAt.IsZero() {
			run.CreatedAt = now
		}
	} else if run.CreatedAt.IsZero() {
		run.CreatedAt = prev.CreatedAt
	}
	if run.UpdatedAt.IsZero() {
		run.UpdatedAt = now
	}
	s.data[run.RunID] = run
	return nil
}

func (s *InMemoryStore) Get(_ context.Context, runID string) (Run, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	run, ok := s.data[runID]
	if !ok {
		return Run{}, false, nil
	}
	return run, true, nil
}
