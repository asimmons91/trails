package jobs

import "context"

// Backend persists an Enqueued job so it can be run later. Concrete
// implementations live in subpackages of jobs/backend, which import jobs
// to satisfy this interface (never the other way around).
type Backend interface {
	Enqueue(ctx context.Context, e Enqueued) error
	Close() error
}

type Runner interface {
	Run(ctx context.Context) error
}
