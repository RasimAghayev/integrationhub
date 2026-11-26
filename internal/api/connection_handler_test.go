package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RasimAghayev/integrationhub/internal/domain"
	"github.com/RasimAghayev/integrationhub/internal/secret"
	"github.com/go-chi/chi/v5"
)

type fakeStore struct {
	items map[string]*domain.Connection
	seq   int
}

func newFakeStore() *fakeStore { return &fakeStore{items: map[string]*domain.Connection{}} }

func (f *fakeStore) Create(_ context.Context, c *domain.Connection) error {
	f.seq++
	c.ID = "id-" + string(rune('0'+f.seq))
	c.CreatedAt = c.UpdatedAt
	f.items[c.ID] = c
	return nil
}
func (f *fakeStore) Get(_ context.Context, id string) (*domain.Connection, error) {
	c, ok := f.items[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return c, nil
}
func (f *fakeStore) List(_ context.Context) ([]*domain.Connection, error) {
	var out []*domain.Connection
	for _, c := range f.items {
		out = append(out, c)
	}
	return out, nil
}
func (f *fakeStore) Update(_ context.Context, c *domain.Connection) error {
	if _, ok := f.items[c.ID]; !ok {
		return domain.ErrNotFound
	}
	f.items[c.ID] = c
	return nil
}
func (f *fakeStore) Delete(_ context.Context, id string) error {
	if _, ok := f.items[id]; !ok {
		return domain.ErrNotFound
	}
	delete(f.items, id)
	return nil
}

func testRouter(h *ConnectionHandler) http.Handler {
	r := chi.NewRouter()
	r.Route("/api/v1/connections", func(r chi.Router) {
		r.Get("/", h.List)
		r.Post("/", h.Create)
		r.Get("/{id}", h.Get)
		r.Put("/{id}", h.Update)
		r.Delete("/{id}", h.Delete)
		r.Post("/{id}/test", h.Test)
		r.Get("/{id}/oauth/authorize", h.OAuthAuthorize)
		r.Get("/{id}/oauth/callback", h.OAuthCallback)
		r.Post("/{id}/oauth/refresh", h.OAuthRefresh)
	})
	return r
}

func do(t *testing.T, r http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *bytes.Reader
	if body == "" {
		rdr = bytes.NewReader(nil)
	} else {
		rdr = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, path, rdr)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestCreateAndGet(t *testing.T) {
	cipher, _ := secret.New(bytes.Repeat([]byte{0x07}, 32))
	h := NewConnectionHandler(newFakeStore(), cipher)
	router := testRouter(h)

	body := `{"name":"GitHub","type":"rest","auth_type":"oauth2","config":{"base_url":"https://api.github.com"},"credentials":{"token":"ghp_secret"}}`
	rec := do(t, router, http.MethodPost, "/api/v1/connections", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var created domain.Connection
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected generated ID")
	}
	if created.SecretRef != "" {
		t.Fatal("secret_ref must not be serialized")
	}

	rec = do(t, router, http.MethodGet, "/api/v1/connections/"+created.ID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d", rec.Code)
	}
}

func TestCreateValidation(t *testing.T) {
	h := NewConnectionHandler(newFakeStore(), nil)
	rec := do(t, testRouter(h), http.MethodPost, "/api/v1/connections", `{"type":"rest"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestGetNotFound(t *testing.T) {
	h := NewConnectionHandler(newFakeStore(), nil)
	rec := do(t, testRouter(h), http.MethodGet, "/api/v1/connections/missing", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestDeleteThenGet(t *testing.T) {
	h := NewConnectionHandler(newFakeStore(), nil)
	router := testRouter(h)
	rec := do(t, router, http.MethodPost, "/api/v1/connections", `{"name":"X","type":"rest","auth_type":"none"}`)
	var created domain.Connection
	_ = json.Unmarshal(rec.Body.Bytes(), &created)

	if rec = do(t, router, http.MethodDelete, "/api/v1/connections/"+created.ID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", rec.Code)
	}
	if rec = do(t, router, http.MethodGet, "/api/v1/connections/"+created.ID, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("get after delete = %d, want 404", rec.Code)
	}
}

func TestCreateRejectsCredentialsWithoutCipher(t *testing.T) {
	h := NewConnectionHandler(newFakeStore(), nil)
	rec := do(t, testRouter(h), http.MethodPost, "/api/v1/connections",
		`{"name":"X","type":"rest","auth_type":"api_key","credentials":{"key":"v"}}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestConnectionTestEndpoint(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "k-123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	cipher, _ := secret.New(bytes.Repeat([]byte{0x09}, 32))
	enc, _ := cipher.Encrypt([]byte(`{"api_key":"k-123"}`))
	store := newFakeStore()
	store.items["c1"] = &domain.Connection{
		ID: "c1", Name: "X", Type: "rest", AuthType: domain.AuthAPIKey,
		Config:    map[string]any{"base_url": upstream.URL, "health_path": "/health"},
		SecretRef: enc,
	}
	h := NewConnectionHandler(store, cipher)

	rec := do(t, testRouter(h), http.MethodPost, "/api/v1/connections/c1/test", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		OK         bool `json:"ok"`
		StatusCode int  `json:"status_code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !out.OK || out.StatusCode != http.StatusOK {
		t.Fatalf("unexpected result: %+v", out)
	}
}

func TestConnectionTestMissingBaseURL(t *testing.T) {
	store := newFakeStore()
	store.items["c1"] = &domain.Connection{ID: "c1", Name: "X", Type: "rest", AuthType: domain.AuthNone}
	h := NewConnectionHandler(store, nil)
	rec := do(t, testRouter(h), http.MethodPost, "/api/v1/connections/c1/test", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
