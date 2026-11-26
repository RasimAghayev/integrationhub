package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/RasimAghayev/integrationhub/internal/domain"
	"github.com/RasimAghayev/integrationhub/internal/execution"
	"github.com/go-chi/chi/v5"
)

// ExecutionHandler serves execution submission, history and dead-letter
// endpoints.
type ExecutionHandler struct {
	svc  *execution.Service
	ints IntegrationStore
	idem *idempotencyStore
}

// NewExecutionHandler builds an ExecutionHandler.
func NewExecutionHandler(svc *execution.Service, ints IntegrationStore) *ExecutionHandler {
	return &ExecutionHandler{svc: svc, ints: ints, idem: newIdempotencyStore(24 * time.Hour)}
}

type executeRequest struct {
	Endpoint string `json:"endpoint"`
	Body     string `json:"body,omitempty"`
	Async    bool   `json:"async,omitempty"`
}

// Execute triggers an endpoint of an integration. With async=true the call
// returns 202 and the work is queued for the background worker.
func (h *ExecutionHandler) Execute(w http.ResponseWriter, r *http.Request) {
	in, err := h.ints.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeNotFound(w, err)
		return
	}
	var req executeRequest
	if status, msg := decodeJSON(w, r, &req); status != 0 {
		writeError(w, status, msg)
		return
	}

	idemKey := r.Header.Get("Idempotency-Key")
	var idemCacheKey string
	if idemKey != "" {
		idemCacheKey = idemKey + "|" + in.ID + "|" + req.Endpoint
		if e, ok := h.idem.get(idemCacheKey); ok {
			writeJSON(w, e.status, e.body)
			return
		}
	}

	if req.Async {
		e, err := h.svc.Submit(r.Context(), in, req.Endpoint, req.Body)
		if err != nil {
			writeExecutionError(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, e)
		return
	}

	e, err := h.svc.Submit(r.Context(), in, req.Endpoint, req.Body)
	if err != nil {
		writeExecutionError(w, err)
		return
	}
	e, err = h.svc.Run(r.Context(), e)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record execution")
		return
	}
	body := map[string]any{
		"execution":   e,
		"ok":          e.Status == domain.ExecutionSucceeded,
		"status_code": e.StatusCode,
		"attempts":    e.Attempts,
		"response":    e.ResponseBody,
	}
	if idemCacheKey != "" {
		h.idem.set(idemCacheKey, http.StatusOK, body)
	}
	writeJSON(w, http.StatusOK, body)
}

// List returns execution history, optionally filtered by integration_id.
func (h *ExecutionHandler) List(w http.ResponseWriter, r *http.Request) {
	integrationID := r.URL.Query().Get("integration_id")
	limit, offset := 50, 0
	if l, o, ok := parsePagination(r); ok {
		limit, offset = l, o
	}
	items, total, err := h.svc.List(r.Context(), integrationID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list executions")
		return
	}
	writeJSON(w, http.StatusOK, page{Items: items, Limit: limit, Offset: offset, Total: total})
}

// Get returns a single execution.
func (h *ExecutionHandler) Get(w http.ResponseWriter, r *http.Request) {
	e, err := h.svc.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeNotFound(w, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

// Dead lists dead-letter executions.
func (h *ExecutionHandler) Dead(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if l, _, ok := parsePagination(r); ok {
		limit = l
	}
	items, err := h.svc.Dead(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list dead executions")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// Replay requeues a dead execution.
func (h *ExecutionHandler) Replay(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Replay(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeNotFound(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "queued"})
}

// Counts reports execution counts per status.
func (h *ExecutionHandler) Counts(w http.ResponseWriter, r *http.Request) {
	counts, err := h.svc.Counts(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to count executions")
		return
	}
	writeJSON(w, http.StatusOK, counts)
}

func writeExecutionError(w http.ResponseWriter, err error) {
	var unknown *execution.UnknownEndpointError
	switch {
	case errors.As(err, &unknown):
		writeError(w, http.StatusBadRequest, unknown.Error())
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	default:
		writeError(w, http.StatusInternalServerError, "failed to queue execution")
	}
}
