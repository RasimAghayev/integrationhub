package api

import (
	"sync"
	"time"
)

// idempotencyStore caches successful execution results keyed by an
// Idempotency-Key so client retries replay the original response.
type idempotencyStore struct {
	mu  sync.Mutex
	m   map[string]idemEntry
	ttl time.Duration
}

type idemEntry struct {
	status  int
	body    map[string]any
	created time.Time
}

func newIdempotencyStore(ttl time.Duration) *idempotencyStore {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &idempotencyStore{m: map[string]idemEntry{}, ttl: ttl}
}

func (s *idempotencyStore) get(key string) (idemEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[key]
	if !ok {
		return idemEntry{}, false
	}
	if time.Since(e.created) > s.ttl {
		delete(s.m, key)
		return idemEntry{}, false
	}
	return e, true
}

func (s *idempotencyStore) set(key string, status int, body map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = idemEntry{status: status, body: body, created: time.Now()}
}
