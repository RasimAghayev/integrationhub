package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/RasimAghayev/integrationhub/internal/domain"
	"github.com/google/uuid"
)

var (
	// ErrInvalidRefresh is returned for unknown, expired or revoked tokens.
	ErrInvalidRefresh = errors.New("auth: invalid refresh token")
	// ErrRefreshReuse is returned when a revoked token is presented again.
	ErrRefreshReuse = errors.New("auth: refresh token reuse detected")
)

// RefreshRecord is the stored state of a refresh token.
type RefreshRecord struct {
	ID         string
	Family     string
	Subject    string
	Roles      []string
	TokenHash  string
	ExpiresAt  time.Time
	RevokedAt  *time.Time
	ReplacedBy *string
}

// RefreshStore persists refresh tokens.
type RefreshStore interface {
	Create(ctx context.Context, rec *RefreshRecord) error
	GetByHash(ctx context.Context, hash string) (*RefreshRecord, error)
	Revoke(ctx context.Context, id, replacedBy string) error
	RevokeFamily(ctx context.Context, family string) error
}

// RefreshManager issues and rotates opaque refresh tokens. Only hashes are
// stored; presenting a revoked token revokes the whole family.
type RefreshManager struct {
	store RefreshStore
	ttl   time.Duration
	now   func() time.Time
}

// NewRefreshManager builds a RefreshManager. ttl defaults to 30 days.
func NewRefreshManager(store RefreshStore, ttl time.Duration) *RefreshManager {
	if ttl <= 0 {
		ttl = 30 * 24 * time.Hour
	}
	return &RefreshManager{store: store, ttl: ttl, now: time.Now}
}

// Issue creates a token for subject and returns the raw secret.
func (m *RefreshManager) Issue(ctx context.Context, subject string, roles []string) (string, error) {
	return m.issue(ctx, subject, roles, uuid.NewString())
}

func (m *RefreshManager) issue(ctx context.Context, subject string, roles []string, family string) (string, error) {
	raw, err := randomToken()
	if err != nil {
		return "", err
	}
	rec := &RefreshRecord{
		Family:    family,
		Subject:   subject,
		Roles:     roles,
		TokenHash: HashRefresh(raw),
		ExpiresAt: m.now().Add(m.ttl),
	}
	if err := m.store.Create(ctx, rec); err != nil {
		return "", err
	}
	return raw, nil
}

// Rotate validates raw, revokes it and returns a fresh token for the same subject.
func (m *RefreshManager) Rotate(ctx context.Context, raw string) (string, string, []string, error) {
	if raw == "" {
		return "", "", nil, ErrInvalidRefresh
	}
	rec, err := m.store.GetByHash(ctx, HashRefresh(raw))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return "", "", nil, ErrInvalidRefresh
		}
		return "", "", nil, err
	}
	if rec.RevokedAt != nil {
		// The same token was already rotated: treat it as a compromise and
		// revoke every token in the family.
		if err := m.store.RevokeFamily(ctx, rec.Family); err != nil {
			return "", "", nil, err
		}
		return "", "", nil, ErrRefreshReuse
	}
	if !m.now().Before(rec.ExpiresAt) {
		return "", "", nil, ErrInvalidRefresh
	}

	next, err := m.issue(ctx, rec.Subject, rec.Roles, rec.Family)
	if err != nil {
		return "", "", nil, err
	}
	created, err := m.store.GetByHash(ctx, HashRefresh(next))
	if err != nil {
		return "", "", nil, err
	}
	if err := m.store.Revoke(ctx, rec.ID, created.ID); err != nil {
		return "", "", nil, err
	}
	return next, rec.Subject, rec.Roles, nil
}

// TTL reports the configured lifetime.
func (m *RefreshManager) TTL() time.Duration { return m.ttl }

// HashRefresh hashes a raw refresh token for storage and lookup.
func HashRefresh(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("auth: generate refresh token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
