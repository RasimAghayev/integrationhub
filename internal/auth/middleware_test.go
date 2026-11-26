package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RasimAghayev/integrationhub/internal/domain"
)

type fakeKeyStore struct {
	rec *KeyRecord
	err error
}

func (f fakeKeyStore) GetByHash(_ context.Context, _ string) (*KeyRecord, error) {
	return f.rec, f.err
}

func serve(a *Authenticator, req *http.Request) (*httptest.ResponseRecorder, *Identity) {
	var got *Identity
	h := a.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got, _ = IdentityFrom(r.Context())
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec, got
}

func TestMiddlewareJWT(t *testing.T) {
	jwt := NewJWTManager([]byte("secret"), time.Hour, "test")
	tok, _ := jwt.Issue("alice", []string{"admin"})
	a := NewAuthenticator(jwt, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/integrations", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec, id := serve(a, req)

	if rec.Code != http.StatusOK || id == nil {
		t.Fatalf("code %d identity %+v", rec.Code, id)
	}
	if id.Subject != "alice" || id.Method != "jwt" {
		t.Fatalf("unexpected identity: %+v", id)
	}
}

func TestMiddlewareRejectsMissingAndInvalid(t *testing.T) {
	jwt := NewJWTManager([]byte("secret"), time.Hour, "test")
	a := NewAuthenticator(jwt, nil)

	rec, _ := serve(a, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing credentials: code %d", rec.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer nonsense")
	rec, _ = serve(a, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("invalid token: code %d", rec.Code)
	}
}

func TestMiddlewareAPIKey(t *testing.T) {
	rec := &KeyRecord{Subject: "svc", Roles: []string{"operator"}, ExpiresAt: time.Now().Add(time.Hour)}
	a := NewAuthenticator(nil, fakeKeyStore{rec: rec})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "raw-key")
	res, id := serve(a, req)

	if res.Code != http.StatusOK || id == nil {
		t.Fatalf("code %d identity %+v", res.Code, id)
	}
	if id.Method != "api_key" || id.Subject != "svc" {
		t.Fatalf("unexpected identity: %+v", id)
	}
}

func TestMiddlewareAPIKeyExpired(t *testing.T) {
	rec := &KeyRecord{Subject: "svc", ExpiresAt: time.Now().Add(-time.Hour)}
	a := NewAuthenticator(nil, fakeKeyStore{rec: rec})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "raw-key")
	res, _ := serve(a, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expired key: code %d", res.Code)
	}
}

func TestMiddlewareNilAuthenticatorPassesThrough(t *testing.T) {
	var a *Authenticator
	rec, id := serve(a, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || id != nil {
		t.Fatalf("code %d identity %+v", rec.Code, id)
	}
}

func TestMiddlewareAPIKeyNotFound(t *testing.T) {
	a := NewAuthenticator(nil, fakeKeyStore{err: domain.ErrNotFound})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "raw-key")
	rec, _ := serve(a, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unknown key: code %d", rec.Code)
	}
}
