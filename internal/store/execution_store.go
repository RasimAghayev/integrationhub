package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/RasimAghayev/integrationhub/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ExecutionStore persists execution records in PostgreSQL.
type ExecutionStore struct {
	pool *pgxpool.Pool
}

// NewExecutionStore builds an ExecutionStore.
func NewExecutionStore(pool *pgxpool.Pool) *ExecutionStore {
	return &ExecutionStore{pool: pool}
}

const executionColumns = `id, integration_id, endpoint, status, request_body,
	response_body, status_code, attempts, error, created_at, started_at, finished_at`

// Create inserts a queued execution and populates its ID and CreatedAt.
func (s *ExecutionStore) Create(ctx context.Context, e *domain.Execution) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO executions (integration_id, endpoint, status, request_body)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`,
		e.IntegrationID, e.Endpoint, e.Status, e.RequestBody,
	).Scan(&e.ID, &e.CreatedAt)
}

// Get returns an execution by ID.
func (s *ExecutionStore) Get(ctx context.Context, id string) (*domain.Execution, error) {
	return scanExecution(s.pool.QueryRow(ctx, `
		SELECT `+executionColumns+` FROM executions WHERE id = $1`, id))
}

// List returns executions for an integration (all integrations when empty),
// newest first, together with the total count.
func (s *ExecutionStore) List(ctx context.Context, integrationID string, limit, offset int) ([]*domain.Execution, int, error) {
	where := ""
	args := []any{}
	if integrationID != "" {
		where = " WHERE integration_id = $1"
		args = append(args, integrationID)
	}
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM executions`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count executions: %w", err)
	}
	query := `SELECT ` + executionColumns + ` FROM executions` + where + ` ORDER BY created_at DESC LIMIT $` +
		fmt.Sprint(len(args)+1) + ` OFFSET $` + fmt.Sprint(len(args)+2)
	args = append(args, limit, offset)
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list executions: %w", err)
	}
	defer rows.Close()

	var out []*domain.Execution
	for rows.Next() {
		e, err := scanExecutionRow(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}

// ClaimNext atomically moves the oldest queued execution to running and returns
// it. It returns domain.ErrNotFound when no work is pending. SKIP LOCKED lets
// several workers claim different rows concurrently.
func (s *ExecutionStore) ClaimNext(ctx context.Context) (*domain.Execution, error) {
	return scanExecution(s.pool.QueryRow(ctx, `
		UPDATE executions SET status = $1, started_at = now()
		WHERE id = (
			SELECT id FROM executions
			WHERE status = $2
			ORDER BY created_at
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		RETURNING `+executionColumns, domain.ExecutionRunning, domain.ExecutionQueued))
}

// Complete records the terminal outcome of an execution.
func (s *ExecutionStore) Complete(ctx context.Context, id string, status domain.ExecutionStatus, statusCode int, responseBody string, attempts int, errMsg string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE executions
		SET status = $2, status_code = $3, response_body = $4,
		    attempts = $5, error = $6, finished_at = now()
		WHERE id = $1`,
		id, status, statusCode, responseBody, attempts, errMsg)
	if err != nil {
		return fmt.Errorf("store: complete execution: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// Requeue resets an execution to queued so the worker picks it up again.
func (s *ExecutionStore) Requeue(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE executions
		SET status = $1, started_at = NULL, finished_at = NULL, error = ''
		WHERE id = $2`,
		domain.ExecutionQueued, id)
	if err != nil {
		return fmt.Errorf("store: requeue execution: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// ListDead returns dead-letter executions, newest first.
func (s *ExecutionStore) ListDead(ctx context.Context, limit int) ([]*domain.Execution, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+executionColumns+` FROM executions
		WHERE status = $1 ORDER BY created_at DESC LIMIT $2`, domain.ExecutionDead, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list dead executions: %w", err)
	}
	defer rows.Close()

	var out []*domain.Execution
	for rows.Next() {
		e, err := scanExecutionRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// CountByStatus returns the number of executions per status.
func (s *ExecutionStore) CountByStatus(ctx context.Context) (map[domain.ExecutionStatus]int, error) {
	rows, err := s.pool.Query(ctx, `SELECT status, count(*) FROM executions GROUP BY status`)
	if err != nil {
		return nil, fmt.Errorf("store: count executions by status: %w", err)
	}
	defer rows.Close()

	out := map[domain.ExecutionStatus]int{}
	for rows.Next() {
		var status domain.ExecutionStatus
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return nil, fmt.Errorf("store: scan execution count: %w", err)
		}
		out[status] = n
	}
	return out, rows.Err()
}

func scanExecution(row pgx.Row) (*domain.Execution, error) {
	return scanExecutionRow(row)
}

func scanExecutionRow(row rowScanner) (*domain.Execution, error) {
	var e domain.Execution
	if err := row.Scan(&e.ID, &e.IntegrationID, &e.Endpoint, &e.Status, &e.RequestBody,
		&e.ResponseBody, &e.StatusCode, &e.Attempts, &e.Error, &e.CreatedAt,
		&e.StartedAt, &e.FinishedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("store: scan execution: %w", err)
	}
	return &e, nil
}
