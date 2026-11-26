package api

import (
	"context"
	"net/http"
	"time"

	"github.com/RasimAghayev/integrationhub/internal/domain"
	"github.com/RasimAghayev/integrationhub/internal/scheduler"
	"github.com/go-chi/chi/v5"
)

// ScheduleStore is the persistence contract used by ScheduleHandler.
type ScheduleStore interface {
	Create(ctx context.Context, sc *domain.Schedule) error
	Get(ctx context.Context, id string) (*domain.Schedule, error)
	List(ctx context.Context) ([]*domain.Schedule, error)
	Update(ctx context.Context, sc *domain.Schedule) error
	Delete(ctx context.Context, id string) error
}

// ScheduleHandler serves schedule CRUD endpoints.
type ScheduleHandler struct {
	store ScheduleStore
}

// NewScheduleHandler builds a ScheduleHandler.
func NewScheduleHandler(store ScheduleStore) *ScheduleHandler {
	return &ScheduleHandler{store: store}
}

type scheduleRequest struct {
	Name          string `json:"name"`
	IntegrationID string `json:"integration_id"`
	Endpoint      string `json:"endpoint"`
	Cron          string `json:"cron"`
	Enabled       *bool  `json:"enabled"`
}

func (h *ScheduleHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req scheduleRequest
	if status, msg := decodeJSON(w, r, &req); status != 0 {
		writeError(w, status, msg)
		return
	}
	sc := &domain.Schedule{
		Name:          req.Name,
		IntegrationID: req.IntegrationID,
		Endpoint:      req.Endpoint,
		Cron:          req.Cron,
		Enabled:       true,
	}
	if req.Enabled != nil {
		sc.Enabled = *req.Enabled
	}
	if err := sc.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	next, err := scheduler.Next(sc.Cron, time.Now())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid cron expression")
		return
	}
	sc.NextRun = next
	if err := h.store.Create(r.Context(), sc); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create schedule")
		return
	}
	writeJSON(w, http.StatusCreated, sc)
}

func (h *ScheduleHandler) List(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list schedules")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *ScheduleHandler) Get(w http.ResponseWriter, r *http.Request) {
	sc, err := h.store.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeNotFound(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sc)
}

func (h *ScheduleHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	existing, err := h.store.Get(r.Context(), id)
	if err != nil {
		writeNotFound(w, err)
		return
	}
	var req scheduleRequest
	if status, msg := decodeJSON(w, r, &req); status != 0 {
		writeError(w, status, msg)
		return
	}
	existing.Name = req.Name
	existing.IntegrationID = req.IntegrationID
	existing.Endpoint = req.Endpoint
	existing.Cron = req.Cron
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}
	if err := existing.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	next, err := scheduler.Next(existing.Cron, time.Now())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid cron expression")
		return
	}
	existing.NextRun = next
	if err := h.store.Update(r.Context(), existing); err != nil {
		writeNotFound(w, err)
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

func (h *ScheduleHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.store.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeNotFound(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
