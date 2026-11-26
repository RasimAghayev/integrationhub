package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/RasimAghayev/integrationhub/internal/domain"
	"github.com/go-chi/chi/v5"
)

type memScheduleStore struct {
	items map[string]*domain.Schedule
	seq   int
}

func newMemScheduleStore() *memScheduleStore {
	return &memScheduleStore{items: map[string]*domain.Schedule{}}
}

func (m *memScheduleStore) Create(_ context.Context, sc *domain.Schedule) error {
	m.seq++
	sc.ID = "sched-" + string(rune('0'+m.seq))
	sc.CreatedAt, sc.UpdatedAt = time.Now(), time.Now()
	m.items[sc.ID] = sc
	return nil
}

func (m *memScheduleStore) Get(_ context.Context, id string) (*domain.Schedule, error) {
	sc, ok := m.items[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return sc, nil
}

func (m *memScheduleStore) List(_ context.Context) ([]*domain.Schedule, error) {
	var out []*domain.Schedule
	for _, sc := range m.items {
		out = append(out, sc)
	}
	return out, nil
}

func (m *memScheduleStore) Update(_ context.Context, sc *domain.Schedule) error {
	if _, ok := m.items[sc.ID]; !ok {
		return domain.ErrNotFound
	}
	m.items[sc.ID] = sc
	return nil
}

func (m *memScheduleStore) Delete(_ context.Context, id string) error {
	if _, ok := m.items[id]; !ok {
		return domain.ErrNotFound
	}
	delete(m.items, id)
	return nil
}

func scheduleRouter(h *ScheduleHandler) http.Handler {
	r := chi.NewRouter()
	r.Route("/api/v1/schedules", func(r chi.Router) {
		r.Get("/", h.List)
		r.Post("/", h.Create)
		r.Get("/{id}", h.Get)
		r.Put("/{id}", h.Update)
		r.Delete("/{id}", h.Delete)
	})
	return r
}

func TestScheduleCRUD(t *testing.T) {
	h := NewScheduleHandler(newMemScheduleStore())
	router := scheduleRouter(h)

	rec := doInt(t, router, http.MethodPost, "/api/v1/schedules",
		`{"name":"nightly","integration_id":"int-1","endpoint":"get","cron":"0 3 * * *"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d body=%s", rec.Code, rec.Body.String())
	}
	var created domain.Schedule
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.NextRun.IsZero() || !created.Enabled {
		t.Fatalf("unexpected schedule: %+v", created)
	}

	if rec = doInt(t, router, http.MethodGet, "/api/v1/schedules", ""); rec.Code != http.StatusOK {
		t.Fatalf("list = %d", rec.Code)
	}
	rec = doInt(t, router, http.MethodPut, "/api/v1/schedules/"+created.ID,
		`{"name":"nightly","integration_id":"int-1","endpoint":"get","cron":"*/10 * * * *","enabled":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update = %d body=%s", rec.Code, rec.Body.String())
	}
	if rec = doInt(t, router, http.MethodDelete, "/api/v1/schedules/"+created.ID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", rec.Code)
	}
}

func TestScheduleRejectsInvalidCron(t *testing.T) {
	rec := doInt(t, scheduleRouter(NewScheduleHandler(newMemScheduleStore())),
		http.MethodPost, "/api/v1/schedules",
		`{"name":"bad","integration_id":"int-1","endpoint":"get","cron":"nope"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestScheduleValidation(t *testing.T) {
	rec := doInt(t, scheduleRouter(NewScheduleHandler(newMemScheduleStore())),
		http.MethodPost, "/api/v1/schedules", `{"cron":"* * * * *"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}
