// Package jobs provides a backend-agnostic background job queue: define a
// Job, register it with a Registry, and Enqueue it against any Backend.
// Concrete backends live in subpackages of jobs/backend (each persists and
// eventually runs an Enqueued job its own way); jobs/jobstest provides a
// Backend double for use in tests. RegisterSchedule plus Scheduler add
// cron-like recurring jobs on top of the same Enqueue path.
package jobs

import "context"

// Job is a unit of background work, identified by Kind and executed by
// Perform. Implementations are typically small structs whose exported
// fields Enqueue JSON-marshals as arguments and Registry.Dispatch
// unmarshals again before calling Perform.
type Job interface {
	// Kind identifies this Job's type. It is stored on Enqueued.Kind and
	// used by Registry to find the factory that reconstructs the Job on
	// dispatch, so it must match the kind string passed to
	// Registry.Register.
	Kind() string

	// Perform executes the job. A returned error signals failure to the
	// Backend; whether and how the job is retried is backend-specific (see
	// Enqueued.MaxAttempts).
	Perform(ctx context.Context) error
}
