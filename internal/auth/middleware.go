package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// KeyRecord is the persisted metadata for an API key.
type KeyRecord struct {
	ID        string
	Subject   string
	Roles     []string
	ExpiresAt time.Time
}

// KeyStore resolves API key hashes to records.
type KeyStore interface {
	GetByHash(ctx context.Context, hash string) (*KeyRecord, error)
}

type identityKey struct{}

// Identity is the authenticated caller.
type Identity struct {
	Subject string
	Roles   []string
	Method  string
}

// Authenticator resolves an identity from a JWT or an API key.
type Authenticator struct {
	jwt          *JWTManager
	keys         KeyStore
	apiKeyHeader string
}

// NewAuthenticator builds an Authenticator. jwt may be nil to accept API keys
// only; keys may be nil to accept JWTs only.
func NewAuthenticator(jwt *JWTManager, keys KeyStore) *Authenticator {
	return &Authenticator{jwt: jwt, keys: keys, apiKeyHeader: "X-API-Key"}
}

// Middleware authenticates the request and stores the identity in the context.
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	if a == nil || (a.jwt == nil && a.keys == nil) {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := a.authenticate(r)
		if err != nil {
			writeUnauthorized(w, err)
			return
		}
		ctx := context.WithValue(r.Context(), identityKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *Authenticator) authenticate(r *http.Request) (*Identity, error) {
	if header := r.Header.Get("Authorization"); header != "" {
		if a.jwt == nil {
			return nil, errUnauthorized("jwt authentication disabled")
		}
		raw, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || raw == "" {
			return nil, errUnauthorized("invalid authorization header")
		}
		claims, err := a.jwt.Verify(raw)
		if err != nil {
			return nil, errUnauthorized("invalid or expired token")
		}
		return &Identity{Subject: claims.Subject, Roles: claims.Roles, Method: "jwt"}, nil
	}

	raw := r.Header.Get(a.apiKeyHeader)
	if raw == "" {
		return nil, errUnauthorized("missing credentials")
	}
	if a.keys == nil {
		return nil, errUnauthorized("api key authentication disabled")
	}
	rec, err := a.keys.GetByHash(r.Context(), HashKey(raw))
	if err != nil {
		return nil, errUnauthorized("invalid api key")
	}
	if !rec.ExpiresAt.IsZero() && time.Now().After(rec.ExpiresAt) {
		return nil, errUnauthorized("api key expired")
	}
	return &Identity{Subject: rec.Subject, Roles: rec.Roles, Method: "api_key"}, nil
}

// IdentityFrom returns the authenticated identity, if any.
func IdentityFrom(ctx context.Context) (*Identity, bool) {
	id, ok := ctx.Value(identityKey{}).(*Identity)
	return id, ok
}

type unauthorizedError string

func (e unauthorizedError) Error() string { return string(e) }

func errUnauthorized(msg string) error { return unauthorizedError(msg) }

func writeUnauthorized(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
