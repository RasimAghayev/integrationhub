package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RasimAghayev/integrationhub/internal/auth"
	"github.com/RasimAghayev/integrationhub/internal/domain"
	"github.com/go-chi/chi/v5"
)

type fakeKeyStore struct {
	rec *auth.KeyRecord
}

func (f fakeKeyStore) GetByHash(_ context.Context, _ string) (*auth.KeyRecord, error) {
	if f.rec == nil {
		return nil, domain.ErrNotFound
	}
	return f.rec, nil
}

func authRouter(h *AuthHandler) http.Handler {
	r := chi.NewRouter()
	r.Post("/api/v1/auth/token", h.Token)
	r.Post("/api/v1/auth/refresh", h.Refresh)
	return r
}

func TestAuthTokenIssuesRefresh(t *testing.T) {
	jwtMgr := auth.NewJWTManager([]byte("secret"), time.Hour, "test")
	refresh := auth.NewRefreshManager(newMemRefreshStoreForAPI(), time.Hour)
	h := NewAuthHandler(jwtMgr, fakeKeyStore{rec: &auth.KeyRecord{
		Subject: "svc", Roles: []string{"admin"},
	}}, refresh)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/token", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("X-API-Key", "raw")
	rec := httptest.NewRecorder()
	authRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.AccessToken == "" || out.RefreshToken == "" {
		t.Fatalf("tokens missing: %+v", out)
	}
}

func TestAuthRefreshRotates(t *testing.T) {
	jwtMgr := auth.NewJWTManager([]byte("secret"), time.Hour, "test")
	store := newMemRefreshStoreForAPI()
	refresh := auth.NewRefreshManager(store, time.Hour)
	h := NewAuthHandler(jwtMgr, fakeKeyStore{}, refresh)

	raw, err := refresh.Issue(context.Background(), "svc", []string{"admin"})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh",
		bytes.NewReader([]byte(`{"refresh_token":"`+raw+`"}`)))
	rec := httptest.NewRecorder()
	authRouter(h).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.AccessToken == "" || out.RefreshToken == raw {
		t.Fatalf("unexpected tokens: %+v", out)
	}

	// Reusing the rotated token must be rejected.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh",
		bytes.NewReader([]byte(`{"refresh_token":"`+raw+`"}`)))
	rec = httptest.NewRecorder()
	authRouter(h).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("reuse status = %d", rec.Code)
	}
}

func TestAuthRefreshRequiresToken(t *testing.T) {
	jwtMgr := auth.NewJWTManager([]byte("secret"), time.Hour, "test")
	h := NewAuthHandler(jwtMgr, fakeKeyStore{}, auth.NewRefreshManager(newMemRefreshStoreForAPI(), time.Hour))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()
	authRouter(h).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}
