package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/RasimAghayev/integrationhub/internal/auth"
	"github.com/RasimAghayev/integrationhub/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RefreshTokenRecord is one stored refresh token.
type RefreshTokenRecord struct {
	ID         string
	Family     string
	Subject    string
	TokenHash  string
	Roles      []string
	ExpiresAt  time.Time
	RevokedAt  *time.Time
	ReplacedBy *string
}

// RefreshTokenStore persists rotating refresh tokens.
type RefreshTokenStore struct {
	pool *pgxpool.Pool
}

// NewRefreshTokenStore builds a RefreshTokenStore.
func NewRefreshTokenStore(pool *pgxpool.Pool) *RefreshTokenStore {
	return &RefreshTokenStore{pool: pool}
}

// Create inserts a token and populates its ID.
func (s *RefreshTokenStore) Create(ctx context.Context, rec *RefreshTokenRecord) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO refresh_tokens (family, subject, roles, token_hash, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`,
		rec.Family, rec.Subject, rec.Roles, rec.TokenHash, rec.ExpiresAt,
	).Scan(&rec.ID)
}

// GetByHash returns the token with the given hash.
func (s *RefreshTokenStore) GetByHash(ctx context.Context, hash string) (*RefreshTokenRecord, error) {
	var rec RefreshTokenRecord
	err := s.pool.QueryRow(ctx, `
		SELECT id, family, subject, roles, token_hash, expires_at, revoked_at, replaced_by
		FROM refresh_tokens WHERE token_hash = $1`, hash,
	).Scan(&rec.ID, &rec.Family, &rec.Subject, &rec.Roles, &rec.TokenHash, &rec.ExpiresAt, &rec.RevokedAt, &rec.ReplacedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get refresh token: %w", err)
	}
	return &rec, nil
}

// Revoke marks a token revoked and records its replacement.
func (s *RefreshTokenStore) Revoke(ctx context.Context, id string, replacedBy string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE refresh_tokens SET revoked_at = now(), replaced_by = $2 WHERE id = $1`,
		id, replacedBy)
	if err != nil {
		return fmt.Errorf("store: revoke refresh token: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// RevokeFamily revokes every token in a family, used on reuse detection.
func (s *RefreshTokenStore) RevokeFamily(ctx context.Context, family string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE refresh_tokens SET revoked_at = now() WHERE family = $1 AND revoked_at IS NULL`, family)
	if err != nil {
		return fmt.Errorf("store: revoke refresh family: %w", err)
	}
	return nil
}

// DeleteExpired removes tokens past their expiry plus a grace window.
func (s *RefreshTokenStore) DeleteExpired(ctx context.Context, before time.Time) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM refresh_tokens WHERE expires_at < $1`, before)
	if err != nil {
		return fmt.Errorf("store: delete expired refresh tokens: %w", err)
	}
	return nil
}

type authRefreshStore struct{ s *RefreshTokenStore }

// AsAuthStore adapts the store to the interface expected by auth.RefreshManager.
func (s *RefreshTokenStore) AsAuthStore() auth.RefreshStore { return authRefreshStore{s} }

func (a authRefreshStore) Create(ctx context.Context, rec *auth.RefreshRecord) error {
	return a.s.Create(ctx, &RefreshTokenRecord{
		Family: rec.Family, Subject: rec.Subject, Roles: rec.Roles, TokenHash: rec.TokenHash, ExpiresAt: rec.ExpiresAt,
	})
}

func (a authRefreshStore) GetByHash(ctx context.Context, hash string) (*auth.RefreshRecord, error) {
	rec, err := a.s.GetByHash(ctx, hash)
	if err != nil {
		return nil, err
	}
	return &auth.RefreshRecord{
		ID: rec.ID, Family: rec.Family, Subject: rec.Subject, Roles: rec.Roles, TokenHash: rec.TokenHash,
		ExpiresAt: rec.ExpiresAt, RevokedAt: rec.RevokedAt, ReplacedBy: rec.ReplacedBy,
	}, nil
}

func (a authRefreshStore) Revoke(ctx context.Context, id, replacedBy string) error {
	return a.s.Revoke(ctx, id, replacedBy)
}

func (a authRefreshStore) RevokeFamily(ctx context.Context, family string) error {
	return a.s.RevokeFamily(ctx, family)
}
