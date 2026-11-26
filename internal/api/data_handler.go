package api

import (
	"context"
	"net/http"
	"strconv"

	"github.com/RasimAghayev/integrationhub/internal/data"
	"github.com/RasimAghayev/integrationhub/internal/domain"
	"github.com/go-chi/chi/v5"
)

// DataHandler serves schema inference and export over stored executions.
type DataHandler struct {
	svc  executionLister
	ints IntegrationStore
}

// executionLister is the subset of the execution service the data API needs.
type executionLister interface {
	List(ctx context.Context, integrationID string, limit, offset int) ([]*domain.Execution, int, error)
}

// NewDataHandler builds a DataHandler.
func NewDataHandler(svc executionLister, ints IntegrationStore) *DataHandler {
	return &DataHandler{svc: svc, ints: ints}
}

// Schema infers the response schema of an integration from its latest executions.
func (h *DataHandler) Schema(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := h.ints.Get(r.Context(), id); err != nil {
		writeNotFound(w, err)
		return
	}
	items, _, err := h.svc.List(r.Context(), id, 20, 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load executions")
		return
	}
	var sample []byte
	for _, e := range items {
		if e.ResponseBody != "" {
			sample = []byte(e.ResponseBody)
			break
		}
	}
	if len(sample) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"fields": []data.Field{}, "samples": 0})
		return
	}
	fields, err := data.Infer(sample)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"fields": fields, "samples": len(items)})
}

// Export streams stored responses as CSV or JSON.
func (h *DataHandler) Export(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("integration_id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "integration_id is required")
		return
	}
	if _, err := h.ints.Get(r.Context(), id); err != nil {
		writeNotFound(w, err)
		return
	}
	limit := 100
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 1000 {
		limit = v
	}
	items, _, err := h.svc.List(r.Context(), id, limit, 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load executions")
		return
	}
	inputs := make([]data.RecordInput, 0, len(items))
	for _, e := range items {
		inputs = append(inputs, data.RecordInput{
			ExecutionID: e.ID, Status: string(e.Status), StatusCode: e.StatusCode, Body: e.ResponseBody,
		})
	}
	records := data.Records(inputs)

	if r.URL.Query().Get("format") == "csv" {
		body, err := data.ToCSV(records)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to encode csv")
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="executions.csv"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"records": records, "count": len(records)})
}
