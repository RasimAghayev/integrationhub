// Package execution persists endpoint invocations and runs them through the
// executor, either synchronously or via a background worker. Executions that
// exhaust their retries land in a dead-letter queue and can be replayed.
package execution

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/RasimAghayev/integrationhub/internal/connauth"
	"github.com/RasimAghayev/integrationhub/internal/domain"
	"github.com/RasimAghayev/integrationhub/internal/executor"
	"github.com/RasimAghayev/integrationhub/internal/secret"
)

// ExecutionStore is the persistence contract used by Service.
type ExecutionStore interface {
	Create(ctx context.Context, e *domain.Execution) error
	Get(ctx context.Context, id string) (*domain.Execution, error)
	List(ctx context.Context, integrationID string, limit, offset int) ([]*domain.Execution, int, error)
	ClaimNext(ctx context.Context) (*domain.Execution, error)
	Complete(ctx context.Context, id string, status domain.ExecutionStatus, statusCode int, responseBody string, attempts int, errMsg string) error
	Requeue(ctx context.Context, id string) error
	ListDead(ctx context.Context, limit int) ([]*domain.Execution, error)
	CountByStatus(ctx context.Context) (map[domain.ExecutionStatus]int, error)
}

// IntegrationStore is the read contract used to resolve integrations.
type IntegrationStore interface {
	Get(ctx context.Context, id string) (*domain.Integration, error)
}

// ConnectionStore is the read contract used to resolve connections.
type ConnectionStore interface {
	Get(ctx context.Context, id string) (*domain.Connection, error)
}

// Service coordinates persistence and execution.
type Service struct {
	store        ExecutionStore
	integrations IntegrationStore
	conns        ConnectionStore
	exec         *executor.Executor
	cipher       *secret.Cipher
	workers      int
}

// New builds a Service. cipher may be nil when connections carry no secrets.
func New(store ExecutionStore, integrations IntegrationStore, conns ConnectionStore, exec *executor.Executor, cipher *secret.Cipher) *Service {
	return &Service{store: store, integrations: integrations, conns: conns, exec: exec, cipher: cipher, workers: 1}
}

// SetWorkers sets how many executions the background worker runs concurrently.
func (s *Service) SetWorkers(n int) {
	if n > 0 {
		s.workers = n
	}
}

// Submit persists a queued execution for later processing.
func (s *Service) Submit(ctx context.Context, in *domain.Integration, endpoint, body string) (*domain.Execution, error) {
	if !hasEndpoint(in, endpoint) {
		return nil, &UnknownEndpointError{Endpoint: endpoint}
	}
	e := &domain.Execution{
		IntegrationID: in.ID,
		Endpoint:      endpoint,
		Status:        domain.ExecutionQueued,
		RequestBody:   body,
	}
	if err := s.store.Create(ctx, e); err != nil {
		return nil, err
	}
	return e, nil
}

// Run executes a single execution record and persists its terminal outcome.
func (s *Service) Run(ctx context.Context, e *domain.Execution) (*domain.Execution, error) {
	status, statusCode, body, attempts, errMsg := s.perform(ctx, e)
	if err := s.store.Complete(ctx, e.ID, status, statusCode, body, attempts, errMsg); err != nil {
		return nil, err
	}
	e.Status, e.StatusCode, e.ResponseBody, e.Attempts, e.Error = status, statusCode, body, attempts, errMsg
	now := time.Now()
	e.FinishedAt = &now
	return e, nil
}

// Replay moves a dead execution back to the queue.
func (s *Service) Replay(ctx context.Context, id string) error {
	return s.store.Requeue(ctx, id)
}

// Get returns a single execution.
func (s *Service) Get(ctx context.Context, id string) (*domain.Execution, error) {
	return s.store.Get(ctx, id)
}

// List returns executions, optionally scoped to one integration.
func (s *Service) List(ctx context.Context, integrationID string, limit, offset int) ([]*domain.Execution, int, error) {
	return s.store.List(ctx, integrationID, limit, offset)
}

// Dead returns dead-letter executions.
func (s *Service) Dead(ctx context.Context, limit int) ([]*domain.Execution, error) {
	return s.store.ListDead(ctx, limit)
}

// Counts returns execution counts per status.
func (s *Service) Counts(ctx context.Context) (map[domain.ExecutionStatus]int, error) {
	return s.store.CountByStatus(ctx)
}

// StartWorker launches a background loop that claims and runs queued
// executions until ctx is cancelled.
func (s *Service) StartWorker(ctx context.Context, poll time.Duration) {
	if poll <= 0 {
		poll = time.Second
	}
	workers := s.workers
	if workers < 1 {
		workers = 1
	}
	sem := make(chan struct{}, workers)
	go func() {
		ticker := time.NewTicker(poll)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			e, err := s.store.ClaimNext(ctx)
			if err != nil {
				continue
			}
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			go func(e *domain.Execution) {
				defer func() { <-sem }()
				if _, err := s.Run(ctx, e); err != nil {
					slog.Error("execution failed", "id", e.ID, "err", err)
				}
			}(e)
		}
	}()
}

// perform resolves the request, runs it and maps the result to a terminal status.
func (s *Service) perform(ctx context.Context, e *domain.Execution) (domain.ExecutionStatus, int, string, int, string) {
	fail := func(msg string) (domain.ExecutionStatus, int, string, int, string) {
		return domain.ExecutionDead, 0, "", 0, msg
	}

	in, err := s.integrations.Get(ctx, e.IntegrationID)
	if err != nil {
		return fail(err.Error())
	}
	ep := findEndpoint(in, e.Endpoint)
	if ep == nil {
		return fail("unknown endpoint")
	}
	conn, err := s.conns.Get(ctx, in.ConnectionID)
	if err != nil {
		return fail(err.Error())
	}
	creds, err := connauth.Credentials(s.cipher, conn)
	if err != nil {
		return fail("failed to decrypt credentials")
	}

	body := e.RequestBody
	if body == "" {
		body = ep.BodyTemplate
	}
	req := executor.Request{
		Method: ep.Method,
		URL:    strings.TrimRight(connauth.String(conn.Config["base_url"]), "/") + ep.Path,
		Header: http.Header{},
		Body:   []byte(body),
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	connauth.ApplyHeader(req.Header, conn, creds)

	res, err := s.exec.Execute(ctx, conn.ID, in.ID, req)
	status := domain.ExecutionSucceeded
	if err != nil {
		// A 4xx is the caller's fault and must not be retried forever; anything
		// else exhausted its retries and belongs in the dead-letter queue.
		if res != nil && res.StatusCode >= 400 && res.StatusCode < 500 {
			status = domain.ExecutionFailed
		} else {
			status = domain.ExecutionDead
		}
	}

	attempts, statusCode, respBody := 0, 0, ""
	if res != nil {
		attempts, statusCode, respBody = res.Attempts, res.StatusCode, string(res.Body)
	}
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	return status, statusCode, respBody, attempts, errMsg
}

func hasEndpoint(in *domain.Integration, name string) bool {
	return in != nil && findEndpoint(in, name) != nil
}

func findEndpoint(in *domain.Integration, name string) *domain.Endpoint {
	if in == nil {
		return nil
	}
	for i := range in.Endpoints {
		if in.Endpoints[i].Name == name {
			return &in.Endpoints[i]
		}
	}
	return nil
}

// UnknownEndpointError reports a request for an endpoint that does not exist.
type UnknownEndpointError struct{ Endpoint string }

func (e *UnknownEndpointError) Error() string { return "unknown endpoint: " + e.Endpoint }

// ErrNotFound mirrors domain.ErrNotFound for callers that only import execution.
var ErrNotFound = domain.ErrNotFound

// IsNotFound reports whether err means a record was not found.
func IsNotFound(err error) bool { return errors.Is(err, domain.ErrNotFound) }
