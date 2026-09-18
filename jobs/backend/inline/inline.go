// Package inline provides a synchronous jobs.Backend: Enqueue runs the job
// immediately, in the caller's own goroutine, and returns its result. There
// is no queueing, persistence, retry, or concurrency limiting — useful for
// tests and small tools that don't need a real background queue.
package inline

import (
	"context"

	"github.com/asimmons91/trails/jobs"
)

// Backend runs every enqueued job synchronously, in-process, with no
// persistence or retry. See New.
type Backend struct {
	reg *jobs.Registry
}

// New returns a Backend that dispatches jobs through reg.
func New(reg *jobs.Registry) *Backend {
	return &Backend{reg: reg}
}

// Enqueue runs e through the Registry immediately and returns its result;
// the job has already finished by the time Enqueue returns.
func (b *Backend) Enqueue(ctx context.Context, e jobs.Enqueued) error {
	return b.reg.Dispatch(ctx, e)
}

// Close is a no-op: Backend holds no resources. Satisfies jobs.Backend.
func (b *Backend) Close() error { return nil }
