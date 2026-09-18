// Package async provides an in-process, non-durable jobs.Backend: Enqueue
// hands work to a fixed pool of goroutines, with optional per-key
// concurrency limiting and delayed dispatch via Enqueued.ScheduledAt.
// Nothing is persisted, so a process restart drops whatever hasn't run yet.
package async

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/asimmons91/trails/jobs"
)

const (
	defaultQueueSize   = 64
	requeueDelayMin    = 50 * time.Millisecond
	requeueDelayJitter = 150 * time.Millisecond
)

// Option configures a Backend created by New.
type Option func(*Backend)

// WithLogger sets the logger a Backend uses to report job failures (default
// slog.Default()).
func WithLogger(l *slog.Logger) Option {
	return func(b *Backend) { b.logger = l }
}

// WithQueueSize overrides the Backend's internal channel buffer size
// (default 64).
func WithQueueSize(n int) Option {
	return func(b *Backend) { b.queue = make(chan jobs.Enqueued, n) }
}

type slot struct {
	count     int
	expiresAt time.Time
}

// Backend is an in-process jobs.Backend: Enqueue dispatches jobs to a fixed
// pool of worker goroutines started by New. It persists nothing, so an
// unprocessed job is lost if the process exits before a worker runs it.
type Backend struct {
	reg    *jobs.Registry
	logger *slog.Logger

	queue chan jobs.Enqueued
	stop  chan struct{}

	workers  sync.WaitGroup // worker goroutines, for Close
	inFlight sync.WaitGroup // jobs accepted but not yet finished, for Drain

	mu    sync.Mutex
	slots map[string]*slot
}

// New returns a Backend with workers goroutines processing reg's jobs. Call
// Close (or Drain, to wait without stopping workers) during shutdown.
func New(reg *jobs.Registry, workers int, opts ...Option) *Backend {
	b := &Backend{
		reg:    reg,
		logger: slog.Default(),
		queue:  make(chan jobs.Enqueued, defaultQueueSize),
		stop:   make(chan struct{}),
		slots:  make(map[string]*slot),
	}

	for _, opt := range opts {
		opt(b)
	}

	for range workers {
		b.workers.Add(1)
		go b.work()
	}

	return b
}

// Enqueue hands e to a worker goroutine, or — if e.ScheduledAt is in the
// future — schedules it to be handed off later via time.AfterFunc. It
// always returns nil; delivery isn't guaranteed if Close runs first.
func (b *Backend) Enqueue(ctx context.Context, e jobs.Enqueued) error {
	b.inFlight.Add(1)

	delay := time.Until(e.ScheduledAt)
	if e.ScheduledAt.IsZero() || delay <= 0 {
		b.send(e)
		return nil
	}

	time.AfterFunc(delay, func() { b.send(e) })
	return nil
}

func (b *Backend) send(e jobs.Enqueued) {
	select {
	case b.queue <- e:
	case <-b.stop:
		b.inFlight.Done() // dropped: shutting down
	}
}

func (b *Backend) work() {
	defer b.workers.Done()
	for {
		select {
		case e := <-b.queue:
			b.dispatch(e)
		case <-b.stop:
			return
		}
	}
}

// dispatch acquires e's concurrency slot if it has a ConcurrencyKey,
// re-queueing itself after a jittered delay when the slot is full, then
// runs e through the Registry.
func (b *Backend) dispatch(e jobs.Enqueued) {
	if e.ConcurrencyKey != "" && !b.tryAcquire(e.ConcurrencyKey, e.ConcurrencyLimit, e.ConcurrencyDuration) {
		time.AfterFunc(requeueDelay(), func() { b.send(e) })
		return
	}
	if e.ConcurrencyKey != "" {
		defer b.release(e.ConcurrencyKey)
	}
	defer b.inFlight.Done()

	if err := b.reg.Dispatch(context.Background(), e); err != nil {
		b.logger.Error("jobs/async: job failed", "kind", e.Kind, "error", err)
	}
}

func requeueDelay() time.Duration {
	return requeueDelayMin + time.Duration(rand.Int64N(int64(requeueDelayJitter)))
}

func (b *Backend) tryAcquire(key string, limit int, duration time.Duration) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	s, ok := b.slots[key]
	if !ok || now.After(s.expiresAt) {
		s = &slot{}
		b.slots[key] = s
	}

	if s.count >= limit {
		return false
	}

	s.count++
	s.expiresAt = now.Add(duration)
	return true
}

func (b *Backend) release(key string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	s, ok := b.slots[key]
	if !ok {
		return
	}

	s.count--
	if s.count <= 0 {
		delete(b.slots, key)
	}
}

// Drain blocks until every job already accepted by Enqueue has finished, or
// ctx is done. It does not stop new jobs from being enqueued while
// draining.
func (b *Backend) Drain(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		b.inFlight.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close signals every worker to stop and waits for them to exit. Whatever
// job a worker is currently running finishes first, but queued jobs that
// haven't started yet are dropped. Call Drain first to wait for all
// accepted work to finish instead.
func (b *Backend) Close() error {
	close(b.stop)
	b.workers.Wait()
	return nil
}
