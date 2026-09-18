package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// DefaultMaxAttempts is the Enqueued.MaxAttempts value Enqueue applies when
// no WithMaxAttempts option is given.
const DefaultMaxAttempts = 25

// Enqueued is the serialized envelope a Job is turned into for a Backend.
type Enqueued struct {
	// Kind is the Job's type identifier (see Job.Kind), used by Registry to
	// find the factory that reconstructs and runs it.
	Kind string
	// Args is the Job, JSON-marshaled by Enqueue; Registry.Dispatch
	// unmarshals it back into a fresh instance built by the matching
	// factory.
	Args json.RawMessage
	// Queue names the queue this job is enqueued on. Whether, and how, a
	// Backend uses it is backend-specific; empty uses the backend's
	// default.
	Queue string
	// ScheduledAt is when the job should become eligible to run. The zero
	// value means as soon as possible. Set via WithDelay.
	ScheduledAt time.Time
	// MaxAttempts caps how many times a Backend may attempt this job before
	// giving up. Enqueue fills in DefaultMaxAttempts when left zero. Only
	// backends with retry support (dbqueue, cloudtask) honor it.
	MaxAttempts int

	// ConcurrencyKey groups jobs a Backend should limit concurrency across.
	// Empty means unlimited. Enqueue populates this automatically from a
	// ConcurrencyLimited Job.
	ConcurrencyKey string
	// ConcurrencyLimit is the maximum number of jobs sharing ConcurrencyKey
	// a Backend should run within ConcurrencyDuration.
	ConcurrencyLimit int
	// ConcurrencyDuration is the rolling window ConcurrencyLimit applies
	// over.
	ConcurrencyDuration time.Duration
}

// EnqueueOption configures a single Enqueue call.
type EnqueueOption func(*Enqueued)

// WithQueue sets the Enqueued.Queue an Enqueue call uses.
func WithQueue(name string) EnqueueOption {
	return func(e *Enqueued) { e.Queue = name }
}

// WithDelay sets Enqueued.ScheduledAt to d from now, delaying when the job
// becomes eligible to run.
func WithDelay(d time.Duration) EnqueueOption {
	return func(e *Enqueued) { e.ScheduledAt = time.Now().Add(d) }
}

// WithMaxAttempts overrides Enqueued.MaxAttempts for a single Enqueue call,
// in place of DefaultMaxAttempts.
func WithMaxAttempts(n int) EnqueueOption {
	return func(e *Enqueued) { e.MaxAttempts = n }
}

// ConcurrencyLimited is implemented by a Job that wants Enqueue to populate
// Enqueued's concurrency fields automatically.
type ConcurrencyLimited interface {
	// ConcurrencyKey returns the key jobs should be limited together by.
	ConcurrencyKey() string
	// ConcurrencyLimit returns how many jobs sharing ConcurrencyKey may run
	// within duration.
	ConcurrencyLimit() (limit int, duration time.Duration)
}

// Enqueue marshals job to JSON as Args, applies opts (falling back to
// DefaultMaxAttempts for MaxAttempts), fills in the concurrency fields if
// job implements ConcurrencyLimited, and hands the resulting Enqueued to b.
func Enqueue(ctx context.Context, b Backend, job Job, opts ...EnqueueOption) error {
	args, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("jobs: marshaling args for kind %q: %w", job.Kind(), err)
	}

	e := Enqueued{
		Kind: job.Kind(),
		Args: args,
	}

	if cl, ok := job.(ConcurrencyLimited); ok {
		e.ConcurrencyKey = cl.ConcurrencyKey()
		e.ConcurrencyLimit, e.ConcurrencyDuration = cl.ConcurrencyLimit()
	}

	for _, opt := range opts {
		opt(&e)
	}

	if e.MaxAttempts == 0 {
		e.MaxAttempts = DefaultMaxAttempts
	}

	return b.Enqueue(ctx, e)
}
