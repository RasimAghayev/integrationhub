package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/RasimAghayev/integrationhub/internal/domain"
)

type fakeScheduleStore struct {
	due     []*domain.Schedule
	marked  map[string][2]time.Time
	dueErr  error
	markErr error
}

func (f *fakeScheduleStore) Due(_ context.Context, _ time.Time) ([]*domain.Schedule, error) {
	return f.due, f.dueErr
}

func (f *fakeScheduleStore) MarkRun(_ context.Context, id string, last, next time.Time) error {
	if f.marked == nil {
		f.marked = map[string][2]time.Time{}
	}
	f.marked[id] = [2]time.Time{last, next}
	return f.markErr
}

type fakeInts struct{ in *domain.Integration }

func (f fakeInts) Get(_ context.Context, _ string) (*domain.Integration, error) {
	if f.in == nil {
		return nil, domain.ErrNotFound
	}
	return f.in, nil
}

type fakeSubmitter struct {
	calls int
	err   error
}

func (f *fakeSubmitter) Submit(_ context.Context, _ *domain.Integration, _, _ string) (*domain.Execution, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return &domain.Execution{ID: "e1", Status: domain.ExecutionQueued}, nil
}

func TestNext(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	next, err := Next("*/5 * * * *", from)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if next != from.Add(5*time.Minute) {
		t.Fatalf("next = %s", next)
	}
	if _, err := Next("not a cron", from); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestTickEnqueuesDueSchedules(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	store := &fakeScheduleStore{due: []*domain.Schedule{
		{ID: "s1", IntegrationID: "i1", Endpoint: "get", Cron: "*/5 * * * *", Enabled: true},
		{ID: "s2", IntegrationID: "i1", Endpoint: "get", Cron: "bad", Enabled: true},
	}}
	sub := &fakeSubmitter{}
	s := New(store, fakeInts{in: &domain.Integration{ID: "i1"}}, sub, time.Minute)
	s.now = func() time.Time { return now }

	if n := s.Tick(context.Background()); n != 1 {
		t.Fatalf("queued = %d", n)
	}
	if sub.calls != 1 {
		t.Fatalf("submit calls = %d", sub.calls)
	}
	if _, ok := store.marked["s1"]; !ok {
		t.Fatal("s1 must be marked as run")
	}
	if _, ok := store.marked["s2"]; ok {
		t.Fatal("invalid cron must not be marked")
	}
}

func TestTickToleratesStoreErrors(t *testing.T) {
	s := New(&fakeScheduleStore{dueErr: domain.ErrNotFound}, fakeInts{}, &fakeSubmitter{}, time.Minute)
	if n := s.Tick(context.Background()); n != 0 {
		t.Fatalf("queued = %d", n)
	}
}

func TestTickSkipsMissingIntegration(t *testing.T) {
	store := &fakeScheduleStore{due: []*domain.Schedule{
		{ID: "s1", IntegrationID: "i1", Endpoint: "get", Cron: "* * * * *", Enabled: true},
	}}
	sub := &fakeSubmitter{}
	s := New(store, fakeInts{}, sub, time.Minute)
	if n := s.Tick(context.Background()); n != 0 {
		t.Fatalf("queued = %d", n)
	}
	if sub.calls != 0 {
		t.Fatalf("submit calls = %d", sub.calls)
	}
	if _, ok := store.marked["s1"]; !ok {
		t.Fatal("schedule must still advance to avoid a hot loop")
	}
}

func TestNewDefaultsInterval(t *testing.T) {
	s := New(&fakeScheduleStore{}, fakeInts{}, &fakeSubmitter{}, 0)
	if s.interval != time.Minute {
		t.Fatalf("interval = %s", s.interval)
	}
}
