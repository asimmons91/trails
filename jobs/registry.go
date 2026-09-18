package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrUnknownKind is returned, wrapped, by Registry.Dispatch when no factory
// is registered for an Enqueued's Kind.
var ErrUnknownKind = errors.New("jobs: unknown kind")

// Schedule is one recurring entry registered via Registry.RegisterSchedule.
type Schedule struct {
	// Name identifies the schedule; it must be unique within a Registry.
	Name string
	// Interval is the minimum time between runs.
	Interval time.Duration
	// Factory builds a fresh Job to enqueue each time the schedule is due.
	Factory func() Job
	// Options are passed to Enqueue for each run.
	Options []EnqueueOption
}

// Registry maps Job kinds to factories that reconstruct them for Dispatch,
// plus the recurring Schedules a Scheduler drives.
type Registry struct {
	factories map[string]func() Job
	schedules []Schedule
}

// RegistryOption configures a Registry created by NewRegistry.
type RegistryOption func(*Registry)

// NewRegistry returns an empty Registry, ready for Register and
// RegisterSchedule calls.
func NewRegistry(opts ...RegistryOption) *Registry {
	r := &Registry{factories: make(map[string]func() Job)}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Register associates kind with factory, so Dispatch can reconstruct and
// run matching jobs. It panics if kind is already registered.
func (r *Registry) Register(kind string, factory func() Job) {
	if _, exists := r.factories[kind]; exists {
		panic(fmt.Sprintf("jobs: kind %q already registered", kind))
	}
	r.factories[kind] = factory
}

// RegisterSchedule adds a recurring Schedule that a Scheduler enqueues via
// factory every interval, using opts. It panics if name is already
// registered.
func (r *Registry) RegisterSchedule(name string, interval time.Duration, factory func() Job, opts ...EnqueueOption) {
	for _, s := range r.schedules {
		if s.Name == name {
			panic(fmt.Sprintf("jobs: schedule %q already registered", name))
		}
	}
	r.schedules = append(r.schedules, Schedule{
		Name:     name,
		Interval: interval,
		Factory:  factory,
		Options:  opts,
	})
}

// Schedules returns every Schedule registered via RegisterSchedule.
func (r *Registry) Schedules() []Schedule {
	return r.schedules
}

// Dispatch reconstructs and runs the Job for e, using the factory
// registered under e.Kind. It returns ErrUnknownKind if none matches.
func (r *Registry) Dispatch(ctx context.Context, e Enqueued) error {
	factory, ok := r.factories[e.Kind]
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownKind, e.Kind)
	}

	job := factory()
	if len(e.Args) > 0 {
		if err := json.Unmarshal(e.Args, job); err != nil {
			return fmt.Errorf("jobs: unmarshaling args for kind %q: %w", e.Kind, err)
		}
	}

	return job.Perform(ctx)
}
