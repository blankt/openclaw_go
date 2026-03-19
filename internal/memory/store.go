package memory

import (
	"context"
	"sync"
	"time"
)

type Entry struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type Store interface {
	Append(ctx context.Context, runID string, entry Entry) error
	List(ctx context.Context, runID string, limit int) ([]Entry, error)
}

type InMemoryStore struct {
	mu   sync.RWMutex
	data map[string][]Entry
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{data: make(map[string][]Entry)}
}

func (s *InMemoryStore) Append(_ context.Context, runID string, entry Entry) error {
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[runID] = append(s.data[runID], entry)
	return nil
}

func (s *InMemoryStore) List(_ context.Context, runID string, limit int) ([]Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries := s.data[runID]
	if limit > 0 && len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}

	copied := make([]Entry, len(entries))
	copy(copied, entries)
	return copied, nil
}
