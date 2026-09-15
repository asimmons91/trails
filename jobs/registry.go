package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var ErrUnknownKind = errors.New("jobs: unknown kind")

type Schedule struct {
	Name     string
	Interval time.Duration
	Factory  func() Job
	Options  []EnqueueOption
}

type Registry struct {
	factories map[string]func() Job
	schedules []Schedule
}

type RegistryOption func(*Registry)

func NewRegistry(opts ...RegistryOption) *Registry {
	r := &Registry{factories: make(map[string]func() Job)}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

func (r *Registry) Register(kind string, factory func() Job) {
	if _, exists := r.factories[kind]; exists {
		panic(fmt.Sprintf("jobs: kind %q already registered", kind))
	}
	r.factories[kind] = factory
}

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

func (r *Registry) Schedules() []Schedule {
	return r.schedules
}

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
