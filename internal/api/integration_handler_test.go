package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RasimAghayev/integrationhub/internal/domain"
	"github.com/go-chi/chi/v5"
)

type fakeIntegrationStore struct {
	items map[string]*domain.Integration
	seq   int
}

func newFakeIntegrationStore() *fakeIntegrationStore {
	return &fakeIntegrationStore{items: map[string]*domain.Integration{}}
}

func (f *fakeIntegrationStore) Create(_ context.Context, i *domain.Integration) error {
	f.seq++
	i.ID = "int-" + string(rune('0'+f.seq))
	i.CreatedAt = i.UpdatedAt
	f.items[i.ID] = i
	return nil
}
func (f *fakeIntegrationStore) Get(_ context.Context, id string) (*domain.Integration, error) {
	i, ok := f.items[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return i, nil
}
func (f *fakeIntegrationStore) List(_ context.Context) ([]*domain.Integration, error) {
	var out []*domain.Integration
	for _, i := range f.items {
		out = append(out, i)
	}
	return out, nil
}
func (f *fakeIntegrationStore) Update(_ context.Context, i *domain.Integration) error {
	if _, ok := f.items[i.ID]; !ok {
		return domain.ErrNotFound
	}
	f.items[i.ID] = i
	return nil
}
func (f *fakeIntegrationStore) Delete(_ context.Context, id string) error {
	if _, ok := f.items[id]; !ok {
		return domain.ErrNotFound
	}
	delete(f.items, id)
	return nil
}

func integrationRouter(h *IntegrationHandler) http.Handler {
	r := chi.NewRouter()
	r.Route("/api/v1/integrations", func(r chi.Router) {
		r.Get("/", h.List)
		r.Post("/", h.Create)
		r.Get("/{id}", h.Get)
		r.Put("/{id}", h.Update)
		r.Delete("/{id}", h.Delete)
		r.Post("/{id}/mapping/preview", h.PreviewMapping)
	})
	return r
}

func doInt(t *testing.T, r http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestIntegrationCRUD(t *testing.T) {
	h := NewIntegrationHandler(newFakeIntegrationStore())
	router := integrationRouter(h)

	body := `{"name":"GitHub","type":"rest","connection_id":"c1","endpoints":[{"name":"list","method":"GET","path":"/users"}]}`
	rec := doInt(t, router, http.MethodPost, "/api/v1/integrations", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d, body=%s", rec.Code, rec.Body.String())
	}
	var created domain.Integration
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.ID == "" || created.Status != domain.StatusActive {
		t.Fatalf("unexpected created: %+v", created)
	}

	if rec = doInt(t, router, http.MethodGet, "/api/v1/integrations/"+created.ID, ""); rec.Code != http.StatusOK {
		t.Fatalf("get = %d", rec.Code)
	}
	if rec = doInt(t, router, http.MethodDelete, "/api/v1/integrations/"+created.ID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", rec.Code)
	}
	if rec = doInt(t, router, http.MethodGet, "/api/v1/integrations/"+created.ID, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("get after delete = %d", rec.Code)
	}
}

func TestIntegrationValidation(t *testing.T) {
	h := NewIntegrationHandler(newFakeIntegrationStore())
	rec := doInt(t, integrationRouter(h), http.MethodPost, "/api/v1/integrations", `{"type":"rest"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestIntegrationMappingPreview(t *testing.T) {
	store := newFakeIntegrationStore()
	h := NewIntegrationHandler(store)
	router := integrationRouter(h)

	body := `{"name":"GitHub","type":"rest","connection_id":"c1",` +
		`"mappings":[{"source":"data.id","target":"id","transform":"to_int","required":true}],` +
		`"transform":{"type":"none"}}`
	rec := doInt(t, router, http.MethodPost, "/api/v1/integrations", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d, body=%s", rec.Code, rec.Body.String())
	}
	var created domain.Integration
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(created.Mappings) != 1 || created.Mappings[0].Target != "id" {
		t.Fatalf("mappings not persisted: %+v", created.Mappings)
	}

	rec = doInt(t, router, http.MethodPost, "/api/v1/integrations/"+created.ID+"/mapping/preview",
		`{"payload":{"data":{"id":"7"}}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview = %d, body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Output map[string]any `json:"output"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if out.Output["id"] != float64(7) {
		t.Fatalf("mapped output = %#v", out.Output)
	}

	rec = doInt(t, router, http.MethodPost, "/api/v1/integrations/"+created.ID+"/mapping/preview", `{"payload":{}}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("preview without required field = %d", rec.Code)
	}
}

func TestIntegrationRejectsDuplicateMappingTarget(t *testing.T) {
	rec := doInt(t, integrationRouter(NewIntegrationHandler(newFakeIntegrationStore())),
		http.MethodPost, "/api/v1/integrations",
		`{"name":"x","type":"rest","connection_id":"c1",`+
			`"mappings":[{"source":"a","target":"b"},{"source":"c","target":"b"}]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
