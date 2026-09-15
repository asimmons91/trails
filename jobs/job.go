package jobs

import "context"

type Job interface {
	Kind() string
	Perform(ctx context.Context) error
}
