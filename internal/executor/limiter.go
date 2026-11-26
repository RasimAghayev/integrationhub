package executor

import (
	"sync"

	"golang.org/x/time/rate"
)

// limiterRegistry keeps one token-bucket limiter per connection.
type limiterRegistry struct {
	mu    sync.Mutex
	m     map[string]*rate.Limiter
	rate  rate.Limit
	burst int
}

func newLimiterRegistry(r rate.Limit, burst int) *limiterRegistry {
	if burst <= 0 {
		burst = 1
	}
	return &limiterRegistry{m: map[string]*rate.Limiter{}, rate: r, burst: burst}
}

func (r *limiterRegistry) get(key string) *rate.Limiter {
	r.mu.Lock()
	defer r.mu.Unlock()
	l, ok := r.m[key]
	if !ok {
		l = rate.NewLimiter(r.rate, r.burst)
		r.m[key] = l
	}
	return l
}
