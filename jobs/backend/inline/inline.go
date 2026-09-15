package inline

import (
	"context"

	"github.com/asimmons91/trails/jobs"
)

type Backend struct {
	reg *jobs.Registry
}

func New(reg *jobs.Registry) *Backend {
	return &Backend{reg: reg}
}

func (b *Backend) Enqueue(ctx context.Context, e jobs.Enqueued) error {
	return b.reg.Dispatch(ctx, e)
}

func (b *Backend) Close() error { return nil }
