package executor

import (
	"errors"
	"sync"
	"time"
)

// ErrOpen is returned when the circuit breaker rejects a call.
var ErrOpen = errors.New("executor: circuit breaker open")

type breakerState int

const (
	stateClosed breakerState = iota
	stateOpen
	stateHalfOpen
)

// Breaker is a per-dependency circuit breaker (closed -> open -> half-open).
// It is safe for concurrent use.
type Breaker struct {
	mu        sync.Mutex
	state     breakerState
	failures  int
	threshold int
	cooldown  time.Duration
	openedAt  time.Time
	probing   bool
	now       func() time.Time
}

// NewBreaker builds a breaker that opens after threshold consecutive failures
// and stays open for cooldown before allowing a single half-open probe.
func NewBreaker(threshold int, cooldown time.Duration) *Breaker {
	if threshold <= 0 {
		threshold = 5
	}
	if cooldown <= 0 {
		cooldown = 30 * time.Second
	}
	return &Breaker{threshold: threshold, cooldown: cooldown, now: time.Now}
}

// Allow reports whether a call may proceed. It returns ErrOpen when the breaker
// is open or a half-open probe is already in flight.
func (b *Breaker) Allow() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refreshLocked()
	switch b.state {
	case stateOpen:
		return ErrOpen
	case stateHalfOpen:
		if b.probing {
			return ErrOpen
		}
		b.probing = true
		return nil
	default:
		return nil
	}
}

// Success records a successful call and closes the breaker.
func (b *Breaker) Success() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refreshLocked()
	b.failures = 0
	b.probing = false
	b.state = stateClosed
}

// Failure records a failed call and opens the breaker when the threshold is hit.
func (b *Breaker) Failure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refreshLocked()
	if b.state == stateHalfOpen {
		b.state = stateOpen
		b.openedAt = b.now()
		b.probing = false
		return
	}
	b.failures++
	if b.failures >= b.threshold {
		b.state = stateOpen
		b.openedAt = b.now()
	}
}

// State returns the current state name (for metrics/logs).
func (b *Breaker) State() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refreshLocked()
	switch b.state {
	case stateOpen:
		return "open"
	case stateHalfOpen:
		return "half-open"
	default:
		return "closed"
	}
}

func (b *Breaker) refreshLocked() {
	if b.state == stateOpen && b.now().Sub(b.openedAt) >= b.cooldown {
		b.state = stateHalfOpen
		b.probing = false
	}
}

// breakerRegistry keeps one breaker per connection.
type breakerRegistry struct {
	mu        sync.Mutex
	m         map[string]*Breaker
	threshold int
	cooldown  time.Duration
}

func newBreakerRegistry(threshold int, cooldown time.Duration) *breakerRegistry {
	return &breakerRegistry{m: map[string]*Breaker{}, threshold: threshold, cooldown: cooldown}
}

func (r *breakerRegistry) get(key string) *Breaker {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.m[key]
	if !ok {
		b = NewBreaker(r.threshold, r.cooldown)
		r.m[key] = b
	}
	return b
}
