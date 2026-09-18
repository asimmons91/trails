package dbqueue

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	trails "github.com/asimmons91/trails"
	"github.com/asimmons91/trails/jobs"
	"github.com/asimmons91/trails/pack"
)

const (
	defaultPollInterval     = time.Second
	defaultBatchSize        = 10
	defaultWorkers          = 5
	defaultRetention        = 24 * time.Hour
	defaultCleanupInterval  = 5 * time.Minute
	defaultCleanupBatchSize = 500

	defaultBackoffBase = 5 * time.Second
	defaultBackoffMax  = 30 * time.Minute
)

// Option configures a Backend created by New.
type Option func(*Backend)

// WithQueues restricts Run to claiming rows from these queue names
// (default: just "default"). A row enqueued with an empty Queue is stored
// under "default".
func WithQueues(names ...string) Option {
	return func(b *Backend) { b.queues = names }
}

// WithPollInterval sets how often Run checks for newly-available rows
// (default 1s).
func WithPollInterval(d time.Duration) Option {
	return func(b *Backend) { b.pollInterval = d }
}

// WithBatchSize sets the maximum number of rows Run claims per poll
// (default 10).
func WithBatchSize(n int) Option {
	return func(b *Backend) { b.batchSize = n }
}

// WithWorkers sets the maximum number of jobs Run dispatches concurrently
// (default 5).
func WithWorkers(n int) Option {
	return func(b *Backend) { b.workers = n }
}

// WithWorkerID overrides the identifier a Backend records against rows it
// locks (default "<hostname>:<pid>"), used only for diagnostics.
func WithWorkerID(id string) Option {
	return func(b *Backend) { b.workerID = id }
}

// WithRetention sets how long a finished row is kept before Run's periodic
// cleanup deletes it (default 24h).
func WithRetention(d time.Duration) Option {
	return func(b *Backend) { b.retention = d }
}

// WithCleanupInterval sets how often Run deletes rows past WithRetention's
// window (default 5m).
func WithCleanupInterval(d time.Duration) Option {
	return func(b *Backend) { b.cleanupInterval = d }
}

// WithCleanupBatchSize sets the maximum number of rows deleted per cleanup
// pass (default 500).
func WithCleanupBatchSize(n int) Option {
	return func(b *Backend) { b.cleanupBatchSize = n }
}

// WithLogger sets the logger a Backend uses to report claim, dispatch, and
// cleanup errors (default slog.Default()).
func WithLogger(l *slog.Logger) Option {
	return func(b *Backend) { b.logger = l }
}

// WithBackoff sets the base and max of the exponential, jittered backoff
// Run waits before retrying a failed job (default 5s base, 30m max).
func WithBackoff(base, max time.Duration) Option {
	return func(b *Backend) { b.backoffBase, b.backoffMax = base, max }
}

var (
	_ jobs.Backend  = (*Backend)(nil)
	_ trails.Runner = (*Backend)(nil)
)

// Backend is a jobs.Backend that persists jobs as rows in a SQL table.
// Construct one with New, then start Run (typically as a trails.Runner) to
// actually process what's enqueued.
type Backend struct {
	db  *pack.DB
	reg *jobs.Registry

	queues           []string
	pollInterval     time.Duration
	batchSize        int
	workers          int
	workerID         string
	retention        time.Duration
	cleanupInterval  time.Duration
	cleanupBatchSize int
	backoffBase      time.Duration
	backoffMax       time.Duration
	logger           *slog.Logger
}

// New returns a Backend that persists jobs to db and, once Run is started,
// dispatches them through reg. Apply Migration against db before use.
func New(db *pack.DB, reg *jobs.Registry, opts ...Option) *Backend {
	b := &Backend{
		db:               db,
		reg:              reg,
		queues:           []string{defaultQueue},
		pollInterval:     defaultPollInterval,
		batchSize:        defaultBatchSize,
		workers:          defaultWorkers,
		workerID:         defaultWorkerID(),
		retention:        defaultRetention,
		cleanupInterval:  defaultCleanupInterval,
		cleanupBatchSize: defaultCleanupBatchSize,
		backoffBase:      defaultBackoffBase,
		backoffMax:       defaultBackoffMax,
		logger:           slog.Default(),
	}

	for _, opt := range opts {
		opt(b)
	}

	return b
}

func defaultWorkerID() string {
	host, err := os.Hostname()
	if err != nil {
		host = "unknown"
	}
	return fmt.Sprintf("%s:%d", host, os.Getpid())
}

// Enqueue persists e as a new available row. Satisfies jobs.Backend.
func (b *Backend) Enqueue(ctx context.Context, e jobs.Enqueued) error {
	queue := e.Queue
	if queue == "" {
		queue = defaultQueue
	}

	scheduledAt := time.Now()
	if !e.ScheduledAt.IsZero() {
		scheduledAt = e.ScheduledAt
	}

	maxAttempts := e.MaxAttempts
	if maxAttempts == 0 {
		maxAttempts = jobs.DefaultMaxAttempts
	}

	row := &jobRow{
		Kind:                  e.Kind,
		Args:                  string(e.Args),
		Queue:                 queue,
		State:                 stateAvailable,
		ScheduledAt:           scheduledAt.UnixNano(),
		MaxAttempts:           maxAttempts,
		ConcurrencyKey:        e.ConcurrencyKey,
		ConcurrencyLimit:      e.ConcurrencyLimit,
		ConcurrencyDurationNs: int64(e.ConcurrencyDuration),
		CreatedAt:             time.Now().UnixNano(),
	}

	if err := pack.Create(ctx, b.db, row); err != nil {
		return fmt.Errorf("dbqueue: enqueue kind %q: %w", e.Kind, err)
	}

	return nil
}

// Close is a no-op: shutdown is driven by canceling the context passed to
// Run, which then waits for in-flight dispatches to finish before
// returning. Satisfies jobs.Backend.
func (b *Backend) Close() error { return nil }
