package executor

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/time/rate"
)

type fakeDoer struct {
	fn func(*http.Request) (*http.Response, error)
}

func (f fakeDoer) Do(r *http.Request) (*http.Response, error) { return f.fn(r) }

func resp(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func testConfig() Config {
	cfg := DefaultConfig()
	cfg.BaseDelay = time.Millisecond
	cfg.MaxDelay = 2 * time.Millisecond
	cfg.Rate = rate.Inf
	cfg.Burst = 1
	return cfg
}

func TestRetrySucceedsAfterFailures(t *testing.T) {
	calls := 0
	doer := fakeDoer{fn: func(*http.Request) (*http.Response, error) {
		calls++
		if calls < 3 {
			return resp(http.StatusInternalServerError, "boom"), nil
		}
		return resp(http.StatusOK, `{"ok":true}`), nil
	}}
	e := New(testConfig(), doer)

	res, err := e.Execute(context.Background(), "c1", "i1", Request{Method: "GET", URL: "http://x"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.StatusCode != http.StatusOK || res.Attempts != 3 {
		t.Fatalf("status=%d attempts=%d", res.StatusCode, res.Attempts)
	}
}

func TestNoRetryOnClientError(t *testing.T) {
	calls := 0
	doer := fakeDoer{fn: func(*http.Request) (*http.Response, error) {
		calls++
		return resp(http.StatusBadRequest, "bad"), nil
	}}
	e := New(testConfig(), doer)

	_, err := e.Execute(context.Background(), "c1", "i1", Request{Method: "GET", URL: "http://x"})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestBreakerOpens(t *testing.T) {
	cfg := testConfig()
	cfg.MaxAttempts = 1
	cfg.BreakerThreshold = 2
	cfg.BreakerCooldown = time.Hour
	doer := fakeDoer{fn: func(*http.Request) (*http.Response, error) {
		return resp(http.StatusServiceUnavailable, "down"), nil
	}}
	e := New(cfg, doer)

	_, _ = e.Execute(context.Background(), "c1", "i1", Request{Method: "GET", URL: "http://x"})
	_, _ = e.Execute(context.Background(), "c1", "i1", Request{Method: "GET", URL: "http://x"})

	_, err := e.Execute(context.Background(), "c1", "i1", Request{Method: "GET", URL: "http://x"})
	if !errors.Is(err, ErrOpen) {
		t.Fatalf("expected ErrOpen, got %v", err)
	}
}

func TestIsRetryableStatus(t *testing.T) {
	cases := map[int]bool{
		http.StatusOK:                  false,
		http.StatusBadRequest:          false,
		http.StatusTooManyRequests:     true,
		http.StatusInternalServerError: true,
		http.StatusServiceUnavailable:  true,
	}
	for status, want := range cases {
		if got := isRetryableStatus(status); got != want {
			t.Errorf("isRetryableStatus(%d)=%v want %v", status, got, want)
		}
	}
}

func TestBackoffWithinCap(t *testing.T) {
	base, max := 10*time.Millisecond, 40*time.Millisecond
	for attempt := 0; attempt < 10; attempt++ {
		d := backoff(attempt, base, max)
		if d < 0 || d > max {
			t.Fatalf("attempt %d: delay %v out of [0,%v]", attempt, d, max)
		}
	}
}

func TestMetricsRecorded(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics()
	reg.MustRegister(m.Collectors()...)

	doer := fakeDoer{fn: func(*http.Request) (*http.Response, error) {
		return resp(http.StatusOK, "ok"), nil
	}}
	e := New(testConfig(), doer)
	e.SetMetrics(m)

	if _, err := e.Execute(context.Background(), "c1", "i1", Request{Method: "GET", URL: "http://x"}); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	found := false
	for _, mf := range mfs {
		if mf.GetName() == "integrationhub_executor_requests_total" {
			found = true
		}
	}
	if !found {
		t.Fatal("requests metric not recorded")
	}
}
