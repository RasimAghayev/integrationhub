// Package scheduler fires cron-scheduled integration endpoints.
package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/RasimAghayev/integrationhub/internal/domain"
	"github.com/robfig/cron/v3"
)

// ScheduleStore is the persistence contract used by Scheduler.
type ScheduleStore interface {
	Due(ctx context.Context, now time.Time) ([]*domain.Schedule, error)
	MarkRun(ctx context.Context, id string, last, next time.Time) error
}

// IntegrationStore resolves the integration a schedule points at.
type IntegrationStore interface {
	Get(ctx context.Context, id string) (*domain.Integration, error)
}

// Submitter queues an execution for an integration endpoint.
type Submitter interface {
	Submit(ctx context.Context, in *domain.Integration, endpoint, body string) (*domain.Execution, error)
}

// Scheduler evaluates due schedules on a fixed interval.
type Scheduler struct {
	store    ScheduleStore
	ints     IntegrationStore
	exec     Submitter
	interval time.Duration
	now      func() time.Time
}

// New builds a Scheduler. interval defaults to one minute when non-positive.
func New(store ScheduleStore, ints IntegrationStore, exec Submitter, interval time.Duration) *Scheduler {
	if interval <= 0 {
		interval = time.Minute
	}
	return &Scheduler{store: store, ints: ints, exec: exec, interval: interval, now: time.Now}
}

// Next computes the next fire time after from for a standard 5-field cron spec.
func Next(spec string, from time.Time) (time.Time, error) {
	sched, err := cron.ParseStandard(spec)
	if err != nil {
		return time.Time{}, err
	}
	return sched.Next(from), nil
}

// Start runs the evaluation loop until ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	slog.Info("scheduler started", "interval", s.interval.String())
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if n := s.Tick(ctx); n > 0 {
				slog.Debug("scheduler enqueued executions", "count", n)
			}
		}
	}
}

// Tick enqueues every due schedule and returns how many were queued.
func (s *Scheduler) Tick(ctx context.Context) int {
	now := s.now()
	due, err := s.store.Due(ctx, now)
	if err != nil {
		slog.Warn("scheduler: load due schedules", "error", err)
		return 0
	}
	queued := 0
	for _, sc := range due {
		next, nErr := Next(sc.Cron, now)
		if nErr != nil {
			slog.Warn("scheduler: invalid cron", "schedule_id", sc.ID, "cron", sc.Cron, "error", nErr)
			continue
		}
		in, gErr := s.ints.Get(ctx, sc.IntegrationID)
		if gErr != nil {
			slog.Warn("scheduler: integration not found", "schedule_id", sc.ID, "error", gErr)
		} else if _, sErr := s.exec.Submit(ctx, in, sc.Endpoint, ""); sErr != nil {
			slog.Warn("scheduler: submit failed", "schedule_id", sc.ID, "error", sErr)
		} else {
			queued++
		}
		if mErr := s.store.MarkRun(ctx, sc.ID, now, next); mErr != nil {
			slog.Warn("scheduler: mark run failed", "schedule_id", sc.ID, "error", mErr)
		}
	}
	return queued
}
