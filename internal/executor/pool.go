package executor

import (
	"context"
	"sync"
)

// pool bounds the total number of concurrent executions (worker pool).
type pool struct {
	sem chan struct{}
}

func newPool(size int) *pool {
	if size <= 0 {
		size = 10
	}
	return &pool{sem: make(chan struct{}, size)}
}

func (p *pool) acquire(ctx context.Context) error {
	select {
	case p.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *pool) release() {
	<-p.sem
}

// semRegistry keeps one concurrency semaphore per integration.
type semRegistry struct {
	mu   sync.Mutex
	m    map[string]chan struct{}
	size int
}

func newSemRegistry(size int) *semRegistry {
	if size <= 0 {
		size = 10
	}
	return &semRegistry{m: map[string]chan struct{}{}, size: size}
}

func (r *semRegistry) get(key string) chan struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.m[key]
	if !ok {
		s = make(chan struct{}, r.size)
		r.m[key] = s
	}
	return s
}
