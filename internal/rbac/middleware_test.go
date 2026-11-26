package rbac

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RasimAghayev/integrationhub/internal/auth"
)

func TestMiddleware(t *testing.T) {
	jwt := auth.NewJWTManager([]byte("secret"), time.Hour, "test")
	a := auth.NewAuthenticator(jwt, nil)
	e, err := New(DefaultPolicy)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	resource := func(*http.Request) string { return "integrations" }
	h := a.Middleware(e.Middleware(resource)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	viewer, _ := jwt.Issue("u", []string{"viewer"})
	admin, _ := jwt.Issue("a", []string{"admin"})

	do := func(method, token string) int {
		req := httptest.NewRequest(method, "/api/v1/integrations", nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := do(http.MethodGet, viewer); code != http.StatusOK {
		t.Fatalf("viewer read: %d", code)
	}
	if code := do(http.MethodPost, viewer); code != http.StatusForbidden {
		t.Fatalf("viewer write: %d", code)
	}
	if code := do(http.MethodPost, admin); code != http.StatusOK {
		t.Fatalf("admin write: %d", code)
	}
	if code := do(http.MethodGet, ""); code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", code)
	}
}

func TestMiddlewareNilEnforcerPassesThrough(t *testing.T) {
	var e *Enforcer
	h := e.Middleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code %d", rec.Code)
	}
}
