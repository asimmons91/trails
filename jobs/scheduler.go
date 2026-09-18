package jobs

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

const schedulerTickInterval = time.Second

// Scheduler periodically enqueues each due Schedule in a Registry against a
// Backend. See Run.
type Scheduler struct {
	registry *Registry
	backend  Backend

	mu      sync.Mutex
	nextRun map[string]time.Time
}

// NewScheduler returns a Scheduler that enqueues registry's due Schedules
// against backend.
func NewScheduler(registry *Registry, backend Backend) *Scheduler {
	return &Scheduler{
		registry: registry,
		backend:  backend,
		nextRun:  make(map[string]time.Time),
	}
}

// Run ticks once a second, enqueuing every Schedule that's due, until ctx is
// canceled. It always returns nil.
func (s *Scheduler) Run(ctx context.Context) error {
	ticker := time.NewTicker(schedulerTickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-ticker.C:
			s.tick(ctx, now)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context, now time.Time) {
	for _, sched := range s.registry.Schedules() {
		if !s.due(sched, now) {
			continue
		}

		job := sched.Factory()
		if err := Enqueue(ctx, s.backend, job, sched.Options...); err != nil {
			slog.Default().Error("jobs: scheduled enqueue failed", "schedule", sched.Name, "error", err)
		}
	}
}

func (s *Scheduler) due(sched Schedule, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	next, seen := s.nextRun[sched.Name]
	if seen && now.Before(next) {
		return false
	}

	s.nextRun[sched.Name] = now.Add(sched.Interval)
	return true
}
