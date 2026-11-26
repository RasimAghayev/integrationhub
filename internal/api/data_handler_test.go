package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RasimAghayev/integrationhub/internal/domain"
	"github.com/go-chi/chi/v5"
)

type stubExecLister struct {
	items []*domain.Execution
}

func (s stubExecLister) List(_ context.Context, _ string, limit, _ int) ([]*domain.Execution, int, error) {
	if limit > 0 && len(s.items) > limit {
		return s.items[:limit], len(s.items), nil
	}
	return s.items, len(s.items), nil
}

func dataRouter(h *DataHandler) http.Handler {
	r := chi.NewRouter()
	r.Route("/api/v1/data", func(r chi.Router) {
		r.Get("/export", h.Export)
		r.Get("/schema/{id}", h.Schema)
	})
	return r
}

func TestDataSchema(t *testing.T) {
	ints := newFakeIntegrationStore()
	ints.items["int-1"] = &domain.Integration{ID: "int-1", ConnectionID: "c1"}
	h := NewDataHandler(stubExecLister{items: []*domain.Execution{
		{ID: "e1", ResponseBody: `{"id":1,"name":"x"}`},
	}}, ints)

	rec := httptest.NewRecorder()
	dataRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/data/schema/int-1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !contains(body, `"type":"string"`) || !contains(body, `"type":"integer"`) {
		t.Fatalf("body = %s", body)
	}
}

func TestDataSchemaUnknownIntegration(t *testing.T) {
	h := NewDataHandler(stubExecLister{}, newFakeIntegrationStore())
	rec := httptest.NewRecorder()
	dataRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/data/schema/missing", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestDataExportCSV(t *testing.T) {
	ints := newFakeIntegrationStore()
	ints.items["int-1"] = &domain.Integration{ID: "int-1"}
	h := NewDataHandler(stubExecLister{items: []*domain.Execution{
		{ID: "e1", Status: domain.ExecutionSucceeded, StatusCode: 200, ResponseBody: `{"a":1}`},
	}}, ints)

	rec := httptest.NewRecorder()
	dataRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/data/export?integration_id=int-1&format=csv", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/csv; charset=utf-8" {
		t.Fatalf("content type = %q", ct)
	}
	if body := rec.Body.String(); !contains(body, "execution_id") || !contains(body, "e1") {
		t.Fatalf("body = %s", body)
	}
}

func TestDataExportRequiresIntegration(t *testing.T) {
	h := NewDataHandler(stubExecLister{}, newFakeIntegrationStore())
	rec := httptest.NewRecorder()
	dataRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/data/export", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
