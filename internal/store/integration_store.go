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

// IntegrationStore persists integrations in PostgreSQL.
type IntegrationStore struct {
	pool *pgxpool.Pool
}

// NewIntegrationStore builds an IntegrationStore.
func NewIntegrationStore(pool *pgxpool.Pool) *IntegrationStore {
	return &IntegrationStore{pool: pool}
}

// Create inserts an integration and populates its ID and timestamps.
func (s *IntegrationStore) Create(ctx context.Context, i *domain.Integration) error {
	endpoints, err := json.Marshal(i.Endpoints)
	if err != nil {
		return fmt.Errorf("store: marshal endpoints: %w", err)
	}
	mappings, transform, err := marshalMapping(i)
	if err != nil {
		return err
	}
	return s.pool.QueryRow(ctx, `
		INSERT INTO integrations (name, description, type, status, connection_id, endpoints, mappings, transform)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at`,
		i.Name, i.Description, i.Type, i.Status, i.ConnectionID, endpoints, mappings, transform,
	).Scan(&i.ID, &i.CreatedAt, &i.UpdatedAt)
}

// Get returns an integration by ID.
func (s *IntegrationStore) Get(ctx context.Context, id string) (*domain.Integration, error) {
	return scanIntegration(s.pool.QueryRow(ctx, `
		SELECT id, name, description, type, status, connection_id, endpoints, mappings, transform, created_at, updated_at
		FROM integrations WHERE id = $1`, id))
}

// List returns all integrations ordered by creation time.
func (s *IntegrationStore) List(ctx context.Context) ([]*domain.Integration, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, description, type, status, connection_id, endpoints, mappings, transform, created_at, updated_at
		FROM integrations ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("store: list integrations: %w", err)
	}
	defer rows.Close()

	var out []*domain.Integration
	for rows.Next() {
		i, err := scanIntegrationRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// Update modifies an existing integration.
func (s *IntegrationStore) Update(ctx context.Context, i *domain.Integration) error {
	endpoints, err := json.Marshal(i.Endpoints)
	if err != nil {
		return fmt.Errorf("store: marshal endpoints: %w", err)
	}
	mappings, transform, err := marshalMapping(i)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE integrations
		SET name = $2, description = $3, type = $4, status = $5,
		    connection_id = $6, endpoints = $7, mappings = $8, transform = $9, updated_at = now()
		WHERE id = $1`,
		i.ID, i.Name, i.Description, i.Type, i.Status, i.ConnectionID, endpoints, mappings, transform)
	if err != nil {
		return fmt.Errorf("store: update integration: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// Delete removes an integration by ID.
func (s *IntegrationStore) Delete(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM integrations WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("store: delete integration: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func scanIntegration(row pgx.Row) (*domain.Integration, error) {
	return scanIntegrationRow(row)
}

func marshalMapping(i *domain.Integration) ([]byte, []byte, error) {
	mappings, err := json.Marshal(i.Mappings)
	if err != nil {
		return nil, nil, fmt.Errorf("store: marshal mappings: %w", err)
	}
	transform, err := json.Marshal(i.Transform)
	if err != nil {
		return nil, nil, fmt.Errorf("store: marshal transform: %w", err)
	}
	if len(mappings) == 0 {
		mappings = []byte("[]")
	}
	return mappings, transform, nil
}

func scanIntegrationRow(row rowScanner) (*domain.Integration, error) {
	var i domain.Integration
	var endpoints, mappings, transform []byte
	if err := row.Scan(&i.ID, &i.Name, &i.Description, &i.Type, &i.Status, &i.ConnectionID, &endpoints, &mappings, &transform, &i.CreatedAt, &i.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("store: scan integration: %w", err)
	}
	if len(endpoints) > 0 {
		if err := json.Unmarshal(endpoints, &i.Endpoints); err != nil {
			return nil, fmt.Errorf("store: unmarshal endpoints: %w", err)
		}
	}
	if len(mappings) > 0 {
		if err := json.Unmarshal(mappings, &i.Mappings); err != nil {
			return nil, fmt.Errorf("store: unmarshal mappings: %w", err)
		}
	}
	if len(transform) > 0 {
		if err := json.Unmarshal(transform, &i.Transform); err != nil {
			return nil, fmt.Errorf("store: unmarshal transform: %w", err)
		}
	}
	return &i, nil
}
