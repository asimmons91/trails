package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

const DefaultMaxAttempts = 25

// Enqueued is the serialized envelope a Job is turned into for a Backend.
type Enqueued struct {
	Kind        string
	Args        json.RawMessage
	Queue       string
	ScheduledAt time.Time // zero = ASAP
	MaxAttempts int       // unused until a durable backend exists

	ConcurrencyKey      string // "" = unlimited
	ConcurrencyLimit    int
	ConcurrencyDuration time.Duration
}

type EnqueueOption func(*Enqueued)

func WithQueue(name string) EnqueueOption {
	return func(e *Enqueued) { e.Queue = name }
}

func WithDelay(d time.Duration) EnqueueOption {
	return func(e *Enqueued) { e.ScheduledAt = time.Now().Add(d) }
}

func WithMaxAttempts(n int) EnqueueOption {
	return func(e *Enqueued) { e.MaxAttempts = n }
}

type ConcurrencyLimited interface {
	ConcurrencyKey() string
	ConcurrencyLimit() (limit int, duration time.Duration)
}

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
