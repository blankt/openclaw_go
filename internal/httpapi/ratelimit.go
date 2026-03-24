package httpapi

import (
	"sync"
	"time"
)

type rateLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	now    func() time.Time
	state  map[string]limitState
}

type limitState struct {
	windowStart time.Time
	count       int
}

func newRateLimiter(limit int, window time.Duration, now func() time.Time) *rateLimiter {
	if now == nil {
		now = time.Now
	}
	return &rateLimiter{
		limit:  limit,
		window: window,
		now:    now,
		state:  make(map[string]limitState),
	}
}

func (l *rateLimiter) allow(clientID string) (bool, time.Duration) {
	if l == nil || l.limit <= 0 {
		return true, 0
	}
	if clientID == "" {
		clientID = "unknown"
	}

	now := l.now().UTC()
	windowStart := now.Truncate(l.window)
	windowEnd := windowStart.Add(l.window)

	l.mu.Lock()
	defer l.mu.Unlock()

	for k, v := range l.state {
		if v.windowStart.Before(windowStart.Add(-2 * l.window)) {
			delete(l.state, k)
		}
	}

	st := l.state[clientID]
	if st.windowStart != windowStart {
		st = limitState{windowStart: windowStart}
	}
	if st.count >= l.limit {
		return false, windowEnd.Sub(now)
	}
	st.count++
	l.state[clientID] = st
	return true, 0
}
