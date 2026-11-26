package api

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/RasimAghayev/integrationhub/internal/domain"
	"github.com/RasimAghayev/integrationhub/internal/execution"
	"github.com/RasimAghayev/integrationhub/internal/executor"
	"github.com/go-chi/chi/v5"
	"golang.org/x/time/rate"
)

type memExecStore struct {
	mu    sync.Mutex
	items map[string]*domain.Execution
	seq   int
}

func newMemExecStore() *memExecStore {
	return &memExecStore{items: map[string]*domain.Execution{}}
}

func (m *memExecStore) Create(_ context.Context, e *domain.Execution) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	e.ID = fmt.Sprintf("exec-%d", m.seq)
	e.CreatedAt = time.Now()
	m.items[e.ID] = e
	return nil
}

func (m *memExecStore) Get(_ context.Context, id string) (*domain.Execution, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.items[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return e, nil
}

func (m *memExecStore) List(_ context.Context, integrationID string, _, _ int) ([]*domain.Execution, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*domain.Execution
	for _, e := range m.items {
		if integrationID == "" || e.IntegrationID == integrationID {
			out = append(out, e)
		}
	}
	return out, len(out), nil
}

func (m *memExecStore) ClaimNext(_ context.Context) (*domain.Execution, error) {
	return nil, domain.ErrNotFound
}

func (m *memExecStore) Complete(_ context.Context, id string, status domain.ExecutionStatus, statusCode int, body string, attempts int, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.items[id]
	if !ok {
		return domain.ErrNotFound
	}
	e.Status, e.StatusCode, e.ResponseBody, e.Attempts, e.Error = status, statusCode, body, attempts, errMsg
	return nil
}

func (m *memExecStore) Requeue(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.items[id]
	if !ok {
		return domain.ErrNotFound
	}
	e.Status = domain.ExecutionQueued
	return nil
}

func (m *memExecStore) ListDead(_ context.Context, _ int) ([]*domain.Execution, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*domain.Execution
	for _, e := range m.items {
		if e.Status == domain.ExecutionDead {
			out = append(out, e)
		}
	}
	return out, nil
}

func (m *memExecStore) CountByStatus(_ context.Context) (map[domain.ExecutionStatus]int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[domain.ExecutionStatus]int{}
	for _, e := range m.items {
		out[e.Status]++
	}
	return out, nil
}

type memConnStore struct{ c *domain.Connection }

func (m memConnStore) Get(_ context.Context, _ string) (*domain.Connection, error) {
	return m.c, nil
}

func executionRouter(t *testing.T, upstream http.HandlerFunc) (http.Handler, *fakeIntegrationStore) {
	t.Helper()
	srv := httptest.NewServer(upstream)
	t.Cleanup(srv.Close)

	ints := newFakeIntegrationStore()
	ints.items["int-1"] = &domain.Integration{
		ID:           "int-1",
		Name:         "demo",
		Type:         domain.IntegrationREST,
		Status:       domain.StatusActive,
		ConnectionID: "conn-1",
		Endpoints:    []domain.Endpoint{{Name: "get", Method: http.MethodGet, Path: "/ok"}},
	}

	cfg := executor.DefaultConfig()
	cfg.MaxAttempts = 1
	cfg.BaseDelay = time.Millisecond
	cfg.MaxDelay = time.Millisecond
	cfg.Rate = rate.Inf

	store := newMemExecStore()
	svc := execution.New(store, ints, memConnStore{c: &domain.Connection{
		ID:     "conn-1",
		Config: map[string]any{"base_url": srv.URL},
	}}, executor.New(cfg, nil), nil)
	h := NewExecutionHandler(svc, ints)

	r := chi.NewRouter()
	r.Route("/api/v1/integrations", func(r chi.Router) {
		r.Post("/{id}/execute", h.Execute)
	})
	r.Route("/api/v1/executions", func(r chi.Router) {
		r.Get("/", h.List)
		r.Get("/dead", h.Dead)
		r.Get("/{id}", h.Get)
		r.Post("/{id}/replay", h.Replay)
	})
	return r, ints
}

func TestExecuteSync(t *testing.T) {
	r, _ := executionRouter(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	rec := doInt(t, r, http.MethodPost, "/api/v1/integrations/int-1/execute", `{"endpoint":"get"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"ok":true`)) {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestExecuteAsyncReturnsAccepted(t *testing.T) {
	r, _ := executionRouter(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	rec := doInt(t, r, http.MethodPost, "/api/v1/integrations/int-1/execute", `{"endpoint":"get","async":true}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"status":"queued"`)) {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestExecuteUnknownEndpoint(t *testing.T) {
	r, _ := executionRouter(t, func(w http.ResponseWriter, _ *http.Request) {})
	rec := doInt(t, r, http.MethodPost, "/api/v1/integrations/int-1/execute", `{"endpoint":"nope"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
}

func TestExecutionListAndReplay(t *testing.T) {
	r, _ := executionRouter(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	if rec := doInt(t, r, http.MethodPost, "/api/v1/integrations/int-1/execute", `{"endpoint":"get"}`); rec.Code != http.StatusOK {
		t.Fatalf("execute status %d: %s", rec.Code, rec.Body.String())
	}
	rec := doInt(t, r, http.MethodGet, "/api/v1/executions", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list status %d: %s", rec.Code, rec.Body.String())
	}
	dead := doInt(t, r, http.MethodGet, "/api/v1/executions/dead", "")
	if dead.Code != http.StatusOK || !bytes.Contains(dead.Body.Bytes(), []byte("exec-1")) {
		t.Fatalf("dead list %d: %s", dead.Code, dead.Body.String())
	}
	replay := doInt(t, r, http.MethodPost, "/api/v1/executions/exec-1/replay", "")
	if replay.Code != http.StatusAccepted {
		t.Fatalf("replay status %d: %s", replay.Code, replay.Body.String())
	}
}
