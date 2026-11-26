package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/RasimAghayev/integrationhub/internal/domain"
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
}

// NewIntegrationHandler builds an IntegrationHandler.
func NewIntegrationHandler(store IntegrationStore) *IntegrationHandler {
	return &IntegrationHandler{store: store}
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
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
