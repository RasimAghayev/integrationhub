// Package api contains HTTP handlers for the IntegrationHub REST API.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/RasimAghayev/integrationhub/internal/domain"
	"github.com/RasimAghayev/integrationhub/internal/secret"
	"github.com/go-chi/chi/v5"
)

// ConnectionStore is the persistence contract used by ConnectionHandler.
type ConnectionStore interface {
	Create(ctx context.Context, c *domain.Connection) error
	Get(ctx context.Context, id string) (*domain.Connection, error)
	List(ctx context.Context) ([]*domain.Connection, error)
	Update(ctx context.Context, c *domain.Connection) error
	Delete(ctx context.Context, id string) error
}

// ConnectionHandler serves connection CRUD endpoints.
type ConnectionHandler struct {
	store  ConnectionStore
	cipher *secret.Cipher
}

// NewConnectionHandler builds a ConnectionHandler. cipher may be nil when
// credential encryption is not configured; credentials are then rejected.
func NewConnectionHandler(store ConnectionStore, cipher *secret.Cipher) *ConnectionHandler {
	return &ConnectionHandler{store: store, cipher: cipher}
}

type connectionRequest struct {
	Name        string          `json:"name"`
	Type        string          `json:"type"`
	AuthType    domain.AuthType `json:"auth_type"`
	Config      map[string]any  `json:"config"`
	Credentials json.RawMessage `json:"credentials"`
}

func (h *ConnectionHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req connectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	c := &domain.Connection{
		Name:     req.Name,
		Type:     req.Type,
		AuthType: req.AuthType,
		Config:   req.Config,
	}
	if err := c.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.Credentials) > 0 && string(req.Credentials) != "null" {
		if h.cipher == nil {
			writeError(w, http.StatusServiceUnavailable, "credential encryption not configured")
			return
		}
		enc, err := h.cipher.Encrypt(req.Credentials)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to encrypt credentials")
			return
		}
		c.SecretRef = enc
	}
	if err := h.store.Create(r.Context(), c); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create connection")
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (h *ConnectionHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list connections")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *ConnectionHandler) Get(w http.ResponseWriter, r *http.Request) {
	c, err := h.store.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeNotFound(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *ConnectionHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	existing, err := h.store.Get(r.Context(), id)
	if err != nil {
		writeNotFound(w, err)
		return
	}
	var req connectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	existing.Name = req.Name
	existing.Type = req.Type
	existing.AuthType = req.AuthType
	existing.Config = req.Config
	if err := existing.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.Credentials) > 0 && string(req.Credentials) != "null" {
		if h.cipher == nil {
			writeError(w, http.StatusServiceUnavailable, "credential encryption not configured")
			return
		}
		enc, err := h.cipher.Encrypt(req.Credentials)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to encrypt credentials")
			return
		}
		existing.SecretRef = enc
	}
	if err := h.store.Update(r.Context(), existing); err != nil {
		writeNotFound(w, err)
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

func (h *ConnectionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.store.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeNotFound(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeNotFound(w http.ResponseWriter, err error) {
	if errors.Is(err, domain.ErrNotFound) {
		writeError(w, http.StatusNotFound, "connection not found")
		return
	}
	writeError(w, http.StatusInternalServerError, "internal error")
}
