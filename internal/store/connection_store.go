package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/RasimAghayev/integrationhub/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ConnectionStore persists connections in PostgreSQL.
type ConnectionStore struct {
	pool *pgxpool.Pool
}

// NewConnectionStore builds a ConnectionStore.
func NewConnectionStore(pool *pgxpool.Pool) *ConnectionStore {
	return &ConnectionStore{pool: pool}
}

// Create inserts a connection and populates its ID and timestamps.
func (s *ConnectionStore) Create(ctx context.Context, c *domain.Connection) error {
	config, err := json.Marshal(c.Config)
	if err != nil {
		return fmt.Errorf("store: marshal config: %w", err)
	}
	return s.pool.QueryRow(ctx, `
		INSERT INTO connections (name, type, auth_type, config, secret_ref)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at`,
		c.Name, c.Type, c.AuthType, config, c.SecretRef,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
}

// Get returns a connection by ID.
func (s *ConnectionStore) Get(ctx context.Context, id string) (*domain.Connection, error) {
	return s.scan(s.pool.QueryRow(ctx, `
		SELECT id, name, type, auth_type, config, secret_ref, created_at, updated_at
		FROM connections WHERE id = $1`, id))
}

// List returns all connections ordered by creation time.
func (s *ConnectionStore) List(ctx context.Context) ([]*domain.Connection, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, type, auth_type, config, secret_ref, created_at, updated_at
		FROM connections ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("store: list: %w", err)
	}
	defer rows.Close()

	var out []*domain.Connection
	for rows.Next() {
		c, err := scanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Update modifies an existing connection.
func (s *ConnectionStore) Update(ctx context.Context, c *domain.Connection) error {
	config, err := json.Marshal(c.Config)
	if err != nil {
		return fmt.Errorf("store: marshal config: %w", err)
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE connections
		SET name = $2, type = $3, auth_type = $4, config = $5, secret_ref = $6, updated_at = now()
		WHERE id = $1`,
		c.ID, c.Name, c.Type, c.AuthType, config, c.SecretRef)
	if err != nil {
		return fmt.Errorf("store: update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// Delete removes a connection by ID.
func (s *ConnectionStore) Delete(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM connections WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("store: delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func (s *ConnectionStore) scan(row pgx.Row) (*domain.Connection, error) {
	return scanRow(row)
}

func scanRow(row rowScanner) (*domain.Connection, error) {
	var c domain.Connection
	var config []byte
	if err := row.Scan(&c.ID, &c.Name, &c.Type, &c.AuthType, &config, &c.SecretRef, &c.CreatedAt, &c.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("store: scan: %w", err)
	}
	if len(config) > 0 {
		if err := json.Unmarshal(config, &c.Config); err != nil {
			return nil, fmt.Errorf("store: unmarshal config: %w", err)
		}
	}
	return &c, nil
}
