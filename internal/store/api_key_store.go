package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/RasimAghayev/integrationhub/internal/auth"
	"github.com/RasimAghayev/integrationhub/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// APIKeyStore persists hashed API keys in PostgreSQL.
type APIKeyStore struct {
	pool *pgxpool.Pool
}

// NewAPIKeyStore builds an APIKeyStore.
func NewAPIKeyStore(pool *pgxpool.Pool) *APIKeyStore {
	return &APIKeyStore{pool: pool}
}

// Create inserts an API key and populates its ID and CreatedAt.
func (s *APIKeyStore) Create(ctx context.Context, k *domain.APIKey) error {
	roles, err := json.Marshal(k.Roles)
	if err != nil {
		return fmt.Errorf("store: marshal roles: %w", err)
	}
	var expires any
	if !k.ExpiresAt.IsZero() {
		expires = k.ExpiresAt
	}
	return s.pool.QueryRow(ctx, `
		INSERT INTO api_keys (subject, key_hash, roles, expires_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`,
		k.Subject, k.KeyHash, roles, expires,
	).Scan(&k.ID, &k.CreatedAt)
}

// GetByHash resolves a key hash to its record.
func (s *APIKeyStore) GetByHash(ctx context.Context, hash string) (*auth.KeyRecord, error) {
	var rec auth.KeyRecord
	var roles []byte
	var expires *time.Time
	if err := s.pool.QueryRow(ctx, `
		SELECT id, subject, roles, expires_at FROM api_keys WHERE key_hash = $1`, hash,
	).Scan(&rec.ID, &rec.Subject, &roles, &expires); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("store: get api key: %w", err)
	}
	if expires != nil {
		rec.ExpiresAt = *expires
	}
	if len(roles) > 0 {
		if err := json.Unmarshal(roles, &rec.Roles); err != nil {
			return nil, fmt.Errorf("store: unmarshal roles: %w", err)
		}
	}
	return &rec, nil
}

// List returns API keys for a subject (all subjects when empty).
func (s *APIKeyStore) List(ctx context.Context, subject string) ([]*domain.APIKey, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, subject, key_hash, roles, expires_at, created_at
		FROM api_keys WHERE $1 = '' OR subject = $1 ORDER BY created_at`, subject)
	if err != nil {
		return nil, fmt.Errorf("store: list api keys: %w", err)
	}
	defer rows.Close()

	var out []*domain.APIKey
	for rows.Next() {
		var k domain.APIKey
		var roles []byte
		var expires *time.Time
		if err := rows.Scan(&k.ID, &k.Subject, &k.KeyHash, &roles, &expires, &k.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: scan api key: %w", err)
		}
		if expires != nil {
			k.ExpiresAt = *expires
		}
		if len(roles) > 0 {
			if err := json.Unmarshal(roles, &k.Roles); err != nil {
				return nil, fmt.Errorf("store: unmarshal roles: %w", err)
			}
		}
		out = append(out, &k)
	}
	return out, rows.Err()
}

// Delete removes an API key by ID.
func (s *APIKeyStore) Delete(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM api_keys WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("store: delete api key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}
