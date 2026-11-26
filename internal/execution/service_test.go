package execution

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/RasimAghayev/integrationhub/internal/domain"
	"github.com/RasimAghayev/integrationhub/internal/executor"
	"golang.org/x/time/rate"
)

type fakeExecStore struct {
	mu    sync.Mutex
	items map[string]*domain.Execution
	seq   int
}

func newFakeExecStore() *fakeExecStore {
	return &fakeExecStore{items: map[string]*domain.Execution{}}
}

func (f *fakeExecStore) Create(_ context.Context, e *domain.Execution) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	e.ID = fmt.Sprintf("exec-%d", f.seq)
	e.CreatedAt = time.Now()
	f.items[e.ID] = e
	return nil
}

func (f *fakeExecStore) Get(_ context.Context, id string) (*domain.Execution, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.items[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return e, nil
}

func (f *fakeExecStore) List(_ context.Context, integrationID string, limit, offset int) ([]*domain.Execution, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*domain.Execution
	for _, e := range f.items {
		if integrationID == "" || e.IntegrationID == integrationID {
			out = append(out, e)
		}
	}
	return out, len(out), nil
}

func (f *fakeExecStore) ClaimNext(_ context.Context) (*domain.Execution, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, e := range f.items {
		if e.Status == domain.ExecutionQueued {
			e.Status = domain.ExecutionRunning
			return e, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f *fakeExecStore) Complete(_ context.Context, id string, status domain.ExecutionStatus, statusCode int, body string, attempts int, errMsg string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.items[id]
	if !ok {
		return domain.ErrNotFound
	}
	e.Status, e.StatusCode, e.ResponseBody, e.Attempts, e.Error = status, statusCode, body, attempts, errMsg
	return nil
}

func (f *fakeExecStore) Requeue(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	e, ok := f.items[id]
	if !ok {
		return domain.ErrNotFound
	}
	e.Status = domain.ExecutionQueued
	return nil
}

func (f *fakeExecStore) ListDead(_ context.Context, limit int) ([]*domain.Execution, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*domain.Execution
	for _, e := range f.items {
		if e.Status == domain.ExecutionDead {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeExecStore) CountByStatus(_ context.Context) (map[domain.ExecutionStatus]int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[domain.ExecutionStatus]int{}
	for _, e := range f.items {
		out[e.Status]++
	}
	return out, nil
}

type fakeIntStore struct{ in *domain.Integration }

func (f fakeIntStore) Get(_ context.Context, _ string) (*domain.Integration, error) {
	return f.in, nil
}

type fakeConnStore struct{ c *domain.Connection }

func (f fakeConnStore) Get(_ context.Context, _ string) (*domain.Connection, error) {
	return f.c, nil
}

func testService(t *testing.T, handler http.HandlerFunc) (*Service, *fakeExecStore) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	cfg := executor.DefaultConfig()
	cfg.MaxAttempts = 1
	cfg.BaseDelay = time.Millisecond
	cfg.MaxDelay = time.Millisecond
	cfg.Rate = rate.Inf
	cfg.Workers = 1
	cfg.PerIntegration = 1

	store := newFakeExecStore()
	in := &domain.Integration{
		ID:           "int-1",
		ConnectionID: "conn-1",
		Endpoints:    []domain.Endpoint{{Name: "get", Method: http.MethodGet, Path: "/ok"}},
	}
	conn := &domain.Connection{ID: "conn-1", Config: map[string]any{"base_url": srv.URL}}
	svc := New(store, fakeIntStore{in: in}, fakeConnStore{c: conn}, executor.New(cfg, nil), nil)
	return svc, store
}

func queued(t *testing.T, svc *Service, endpoint string) *domain.Execution {
	t.Helper()
	in := &domain.Integration{
		ID:        "int-1",
		Endpoints: []domain.Endpoint{{Name: endpoint, Method: http.MethodGet, Path: "/ok"}},
	}
	e, err := svc.Submit(context.Background(), in, endpoint, "")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	return e
}

func TestRunSuccess(t *testing.T) {
	svc, _ := testService(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	e := queued(t, svc, "get")
	if _, err := svc.Run(context.Background(), e); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if e.Status != domain.ExecutionSucceeded || e.StatusCode != http.StatusOK {
		t.Fatalf("unexpected result: %+v", e)
	}
	if e.ResponseBody != `{"ok":true}` || e.Attempts != 1 {
		t.Fatalf("unexpected body/attempts: %+v", e)
	}
}

func TestRunClientErrorIsFailed(t *testing.T) {
	svc, _ := testService(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})
	e := queued(t, svc, "get")
	if _, err := svc.Run(context.Background(), e); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if e.Status != domain.ExecutionFailed {
		t.Fatalf("expected failed, got %s", e.Status)
	}
}

func TestRunServerErrorIsDeadLettered(t *testing.T) {
	svc, _ := testService(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	e := queued(t, svc, "get")
	if _, err := svc.Run(context.Background(), e); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if e.Status != domain.ExecutionDead {
		t.Fatalf("expected dead, got %s", e.Status)
	}
}

func TestSubmitUnknownEndpoint(t *testing.T) {
	svc, _ := testService(t, func(w http.ResponseWriter, _ *http.Request) {})
	in := &domain.Integration{ID: "int-1"}
	if _, err := svc.Submit(context.Background(), in, "missing", ""); err == nil {
		t.Fatal("expected error for unknown endpoint")
	}
}

func TestSubmitQueues(t *testing.T) {
	svc, store := testService(t, func(w http.ResponseWriter, _ *http.Request) {})
	in := &domain.Integration{ID: "int-1", Endpoints: []domain.Endpoint{{Name: "get", Method: http.MethodGet, Path: "/ok"}}}
	e, err := svc.Submit(context.Background(), in, "get", "")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if e.Status != domain.ExecutionQueued {
		t.Fatalf("expected queued, got %s", e.Status)
	}
	if _, ok := store.items[e.ID]; !ok {
		t.Fatal("execution was not persisted")
	}
}

func TestReplayRequeues(t *testing.T) {
	svc, _ := testService(t, func(w http.ResponseWriter, _ *http.Request) {})
	e := queued(t, svc, "get")
	if _, err := svc.Run(context.Background(), e); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if err := svc.Replay(context.Background(), e.ID); err != nil {
		t.Fatalf("Replay: %v", err)
	}
	got, err := svc.Get(context.Background(), e.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != domain.ExecutionQueued {
		t.Fatalf("expected queued after replay, got %s", got.Status)
	}
}
