package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/RasimAghayev/integrationhub/internal/domain"
	"github.com/RasimAghayev/integrationhub/internal/executor"
	"github.com/go-chi/chi/v5"
)

// IntegrationStore is the persistence contract used by IntegrationHandler.
type IntegrationStore interface {
	Create(ctx context.Context, i *domain.Integration) error
	Get(ctx context.Context, id string) (*domain.Integration, error)
	List(ctx context.Context) ([]*domain.Integration, error)
	Update(ctx context.Context, i *domain.Integration) error
	Delete(ctx context.Context, id string) error
}

// IntegrationHandler serves integration CRUD endpoints.
type IntegrationHandler struct {
	store IntegrationStore
	exec  *executor.Executor
	conns ConnectionStore
	idem  *idempotencyStore
}

// NewIntegrationHandler builds an IntegrationHandler.
func NewIntegrationHandler(store IntegrationStore) *IntegrationHandler {
	return &IntegrationHandler{store: store}
}

// WithExecutor enables the manual execute endpoint by supplying the execution
// engine and the connection store used to resolve base URLs.
func (h *IntegrationHandler) WithExecutor(exec *executor.Executor, conns ConnectionStore) *IntegrationHandler {
	h.exec = exec
	h.conns = conns
	h.idem = newIdempotencyStore(24 * time.Hour)
	return h
}

type integrationRequest struct {
	Name         string                   `json:"name"`
	Description  string                   `json:"description"`
	Type         domain.IntegrationType   `json:"type"`
	Status       domain.IntegrationStatus `json:"status"`
	ConnectionID string                   `json:"connection_id"`
	Endpoints    []domain.Endpoint        `json:"endpoints"`
}

func (h *IntegrationHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req integrationRequest
	if status, msg := decodeJSON(w, r, &req); status != 0 {
		writeError(w, status, msg)
		return
	}
	i := &domain.Integration{
		Name:         req.Name,
		Description:  req.Description,
		Type:         req.Type,
		Status:       req.Status,
		ConnectionID: req.ConnectionID,
		Endpoints:    req.Endpoints,
	}
	if err := i.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.store.Create(r.Context(), i); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create integration")
		return
	}
	writeJSON(w, http.StatusCreated, i)
}

func (h *IntegrationHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list integrations")
		return
	}
	total := len(items)
	if limit, offset, ok := parsePagination(r); ok {
		if offset > total {
			offset = total
		}
		end := offset + limit
		if end > total {
			end = total
		}
		writeJSON(w, http.StatusOK, page{Items: items[offset:end], Limit: limit, Offset: offset, Total: total})
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *IntegrationHandler) Get(w http.ResponseWriter, r *http.Request) {
	i, err := h.store.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeNotFound(w, err)
		return
	}
	writeJSON(w, http.StatusOK, i)
}

func (h *IntegrationHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	existing, err := h.store.Get(r.Context(), id)
	if err != nil {
		writeNotFound(w, err)
		return
	}
	var req integrationRequest
	if status, msg := decodeJSON(w, r, &req); status != 0 {
		writeError(w, status, msg)
		return
	}
	existing.Name = req.Name
	existing.Description = req.Description
	existing.Type = req.Type
	existing.Status = req.Status
	existing.ConnectionID = req.ConnectionID
	existing.Endpoints = req.Endpoints
	if err := existing.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.store.Update(r.Context(), existing); err != nil {
		writeNotFound(w, err)
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

func (h *IntegrationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.store.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeNotFound(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type executeRequest struct {
	Endpoint string `json:"endpoint"`
}

// Execute manually triggers a single endpoint of an integration.
func (h *IntegrationHandler) Execute(w http.ResponseWriter, r *http.Request) {
	if h.exec == nil || h.conns == nil {
		writeError(w, http.StatusServiceUnavailable, "execution engine not configured")
		return
	}
	id := chi.URLParam(r, "id")
	in, err := h.store.Get(r.Context(), id)
	if err != nil {
		writeNotFound(w, err)
		return
	}
	var req executeRequest
	if status, msg := decodeJSON(w, r, &req); status != 0 {
		writeError(w, status, msg)
		return
	}
	var ep *domain.Endpoint
	for i := range in.Endpoints {
		if in.Endpoints[i].Name == req.Endpoint {
			ep = &in.Endpoints[i]
			break
		}
	}
	if ep == nil {
		writeError(w, http.StatusBadRequest, "unknown endpoint")
		return
	}
	conn, err := h.conns.Get(r.Context(), in.ConnectionID)
	if err != nil {
		writeNotFound(w, err)
		return
	}
	baseURL := strings.TrimRight(stringVal(conn.Config["base_url"]), "/")

	idemKey := r.Header.Get("Idempotency-Key")
	var idemCacheKey string
	if idemKey != "" && h.idem != nil {
		idemCacheKey = idemKey + "|" + in.ID + "|" + req.Endpoint
		if e, ok := h.idem.get(idemCacheKey); ok {
			writeJSON(w, e.status, e.body)
			return
		}
	}

	res, err := h.exec.Execute(r.Context(), conn.ID, in.ID, executor.Request{
		Method: ep.Method,
		URL:    baseURL + ep.Path,
	})
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, executor.ErrOpen) {
			status = http.StatusServiceUnavailable
		}
		body := map[string]any{"ok": false, "error": err.Error()}
		if res != nil {
			body["attempts"] = res.Attempts
			body["latency_ms"] = res.Latency.Milliseconds()
		}
		writeJSON(w, status, body)
		return
	}
	body := map[string]any{
		"ok":          true,
		"status_code": res.StatusCode,
		"attempts":    res.Attempts,
		"latency_ms":  res.Latency.Milliseconds(),
		"body":        string(res.Body),
	}
	if idemCacheKey != "" {
		h.idem.set(idemCacheKey, http.StatusOK, body)
	}
	writeJSON(w, http.StatusOK, body)
}
