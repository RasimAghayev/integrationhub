// Package executor runs integration endpoint calls with retry, backoff, a
// per-connection circuit breaker, per-connection rate limiting, and bounded
// concurrency (global worker pool + per-integration limit).
package executor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

// maxRespBytes caps how much of a response body is read into memory.
const maxRespBytes = 1 << 20 // 1 MiB

// errPermanent marks failures that must never be retried (e.g. bad request).
var errPermanent = errors.New("executor: permanent error")

// Doer is the HTTP client contract (satisfied by *http.Client).
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Config controls executor behaviour.
type Config struct {
	MaxAttempts      int
	BaseDelay        time.Duration
	MaxDelay         time.Duration
	Timeout          time.Duration
	Rate             rate.Limit
	Burst            int
	BreakerThreshold int
	BreakerCooldown  time.Duration
	Workers          int
	PerIntegration   int
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		MaxAttempts:      3,
		BaseDelay:        200 * time.Millisecond,
		MaxDelay:         10 * time.Second,
		Timeout:          10 * time.Second,
		Rate:             rate.Limit(10),
		Burst:            20,
		BreakerThreshold: 5,
		BreakerCooldown:  30 * time.Second,
		Workers:          10,
		PerIntegration:   10,
	}
}

// Executor executes endpoint requests.
type Executor struct {
	cfg      Config
	doer     Doer
	breakers *breakerRegistry
	limiters *limiterRegistry
	pool     *pool
	sems     *semRegistry
	metrics  *Metrics
}

// New builds an Executor. A nil doer defaults to http.DefaultClient.
func New(cfg Config, doer Doer) *Executor {
	if doer == nil {
		doer = http.DefaultClient
	}
	return &Executor{
		cfg:      cfg,
		doer:     doer,
		breakers: newBreakerRegistry(cfg.BreakerThreshold, cfg.BreakerCooldown),
		limiters: newLimiterRegistry(cfg.Rate, cfg.Burst),
		pool:     newPool(cfg.Workers),
		sems:     newSemRegistry(cfg.PerIntegration),
	}
}

// Request is a single endpoint call.
type Request struct {
	Method string
	URL    string
	Header http.Header
	Body   []byte
}

// Result is the outcome of an execution.
type Result struct {
	StatusCode int           `json:"status_code"`
	Body       []byte        `json:"-"`
	Latency    time.Duration `json:"latency_ms"`
	Attempts   int           `json:"attempts"`
	Err        error         `json:"-"`
}

// SetMetrics attaches Prometheus collectors to the executor.
func (e *Executor) SetMetrics(m *Metrics) {
	e.metrics = m
}

// Execute runs req with retry/breaker/rate-limit/concurrency controls. connID
// and intID key the per-connection and per-integration state.
func (e *Executor) Execute(ctx context.Context, connID, intID string, req Request) (res *Result, err error) {
	if err := e.limiters.get(connID).Wait(ctx); err != nil {
		return nil, err
	}

	sem := e.sems.get(intID)
	select {
	case sem <- struct{}{}:
		defer func() { <-sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	if err := e.pool.acquire(ctx); err != nil {
		return nil, err
	}
	defer e.pool.release()

	breaker := e.breakers.get(connID)
	defer func() {
		if e.metrics == nil || res == nil {
			return
		}
		outcome := "error"
		if err == nil {
			outcome = "success"
		}
		e.metrics.Requests.WithLabelValues(intID, outcome).Inc()
		e.metrics.Latency.WithLabelValues(intID).Observe(float64(res.Latency.Milliseconds()))
		open := 0.0
		if breaker.State() == "open" {
			open = 1
		}
		e.metrics.Breakers.WithLabelValues(connID).Set(open)
	}()
	start := time.Now()
	var lastErr error
	var lastStatus int
	var lastBody []byte

	for attempt := 0; attempt < e.cfg.MaxAttempts; attempt++ {
		if err := breaker.Allow(); err != nil {
			return &Result{Attempts: attempt, Err: err}, err
		}

		status, body, err := e.doOnce(ctx, req)
		if err != nil {
			lastErr = err
			if !isRetryableErr(err) {
				breaker.Success()
				return &Result{Attempts: attempt + 1, Latency: time.Since(start), Err: err}, err
			}
			breaker.Failure()
		} else {
			lastStatus, lastBody = status, body
			if status >= 200 && status < 300 {
				breaker.Success()
				return &Result{StatusCode: status, Body: body, Attempts: attempt + 1, Latency: time.Since(start)}, nil
			}
			if !isRetryableStatus(status) {
				// Client error: not the dependency's fault, do not trip the breaker.
				err = fmt.Errorf("%w: unexpected status %d", errPermanent, status)
				return &Result{StatusCode: status, Body: body, Attempts: attempt + 1, Latency: time.Since(start), Err: err}, err
			}
			lastErr = fmt.Errorf("executor: retryable status %d", status)
			breaker.Failure()
		}

		if attempt == e.cfg.MaxAttempts-1 {
			break
		}
		timer := time.NewTimer(backoff(attempt, e.cfg.BaseDelay, e.cfg.MaxDelay))
		select {
		case <-ctx.Done():
			timer.Stop()
			return &Result{StatusCode: lastStatus, Body: lastBody, Attempts: attempt + 1, Latency: time.Since(start), Err: ctx.Err()}, ctx.Err()
		case <-timer.C:
		}
	}

	return &Result{StatusCode: lastStatus, Body: lastBody, Attempts: e.cfg.MaxAttempts, Latency: time.Since(start), Err: lastErr}, lastErr
}

func (e *Executor) doOnce(ctx context.Context, req Request) (int, []byte, error) {
	rctx, cancel := context.WithTimeout(ctx, e.cfg.Timeout)
	defer cancel()

	var body io.Reader
	if req.Body != nil {
		body = bytes.NewReader(req.Body)
	}
	httpReq, err := http.NewRequestWithContext(rctx, req.Method, req.URL, body)
	if err != nil {
		return 0, nil, fmt.Errorf("%w: %v", errPermanent, err)
	}
	for k, vs := range req.Header {
		for _, v := range vs {
			httpReq.Header.Add(k, v)
		}
	}

	resp, err := e.doer.Do(httpReq)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	b, _ := io.ReadAll(io.LimitReader(resp.Body, maxRespBytes))
	return resp.StatusCode, b, nil
}

// isRetryableStatus reports whether an HTTP status is worth retrying.
func isRetryableStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

// isRetryableErr reports whether a transport error is worth retrying.
func isRetryableErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errPermanent) || errors.Is(err, context.Canceled) {
		return false
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return true
	}
	return true
}

// backoff returns a full-jitter delay: uniform random in [0, capped exponential].
func backoff(attempt int, base, max time.Duration) time.Duration {
	d := base << uint(attempt)
	if d <= 0 || d > max {
		d = max
	}
	return time.Duration(rand.Int64N(int64(d) + 1))
}
