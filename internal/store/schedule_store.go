package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/RasimAghayev/integrationhub/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ScheduleStore persists schedules in PostgreSQL.
type ScheduleStore struct {
	pool *pgxpool.Pool
}

// NewScheduleStore builds a ScheduleStore.
func NewScheduleStore(pool *pgxpool.Pool) *ScheduleStore {
	return &ScheduleStore{pool: pool}
}

// Create inserts a schedule and populates its ID and timestamps.
func (s *ScheduleStore) Create(ctx context.Context, sc *domain.Schedule) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO schedules (name, integration_id, endpoint, cron, enabled, next_run)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at`,
		sc.Name, sc.IntegrationID, sc.Endpoint, sc.Cron, sc.Enabled, sc.NextRun,
	).Scan(&sc.ID, &sc.CreatedAt, &sc.UpdatedAt)
}

// Get returns a schedule by ID.
func (s *ScheduleStore) Get(ctx context.Context, id string) (*domain.Schedule, error) {
	return scanSchedule(s.pool.QueryRow(ctx, `
		SELECT id, name, integration_id, endpoint, cron, enabled, next_run, last_run, created_at, updated_at
		FROM schedules WHERE id = $1`, id))
}

// List returns all schedules ordered by creation time.
func (s *ScheduleStore) List(ctx context.Context) ([]*domain.Schedule, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, integration_id, endpoint, cron, enabled, next_run, last_run, created_at, updated_at
		FROM schedules ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("store: list schedules: %w", err)
	}
	defer rows.Close()

	var out []*domain.Schedule
	for rows.Next() {
		sc, err := scanScheduleRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}

// Update modifies an existing schedule.
func (s *ScheduleStore) Update(ctx context.Context, sc *domain.Schedule) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE schedules
		SET name = $2, integration_id = $3, endpoint = $4, cron = $5, enabled = $6,
		    next_run = $7, updated_at = now()
		WHERE id = $1`,
		sc.ID, sc.Name, sc.IntegrationID, sc.Endpoint, sc.Cron, sc.Enabled, sc.NextRun)
	if err != nil {
		return fmt.Errorf("store: update schedule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// Delete removes a schedule by ID.
func (s *ScheduleStore) Delete(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM schedules WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("store: delete schedule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// Due returns enabled schedules whose next_run is at or before now.
func (s *ScheduleStore) Due(ctx context.Context, now time.Time) ([]*domain.Schedule, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, integration_id, endpoint, cron, enabled, next_run, last_run, created_at, updated_at
		FROM schedules
		WHERE enabled AND next_run <= $1
		ORDER BY next_run
		LIMIT 100`, now)
	if err != nil {
		return nil, fmt.Errorf("store: query due schedules: %w", err)
	}
	defer rows.Close()

	var out []*domain.Schedule
	for rows.Next() {
		sc, err := scanScheduleRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}

// MarkRun records the last run time and the next fire time.
func (s *ScheduleStore) MarkRun(ctx context.Context, id string, last, next time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE schedules SET last_run = $2, next_run = $3, updated_at = now() WHERE id = $1`, id, last, next)
	if err != nil {
		return fmt.Errorf("store: mark schedule run: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func scanSchedule(row pgx.Row) (*domain.Schedule, error) {
	return scanScheduleRow(row)
}

func scanScheduleRow(row rowScanner) (*domain.Schedule, error) {
	var sc domain.Schedule
	if err := row.Scan(&sc.ID, &sc.Name, &sc.IntegrationID, &sc.Endpoint, &sc.Cron, &sc.Enabled,
		&sc.NextRun, &sc.LastRun, &sc.CreatedAt, &sc.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("store: scan schedule: %w", err)
	}
	return &sc, nil
}
