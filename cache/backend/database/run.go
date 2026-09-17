package database

import (
	"context"
	"errors"
	"time"

	"github.com/asimmons91/trails/pack"
)

// Run periodically sweeps expired entries until ctx is cancelled, in
// batches of sweepBatchSize so a large backlog doesn't hold a long-running
// DELETE. It satisfies trails.Runner.
func (b *Backend) Run(ctx context.Context) error {
	ticker := time.NewTicker(b.sweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := b.sweep(ctx); err != nil && !errors.Is(err, context.Canceled) {
				b.logger.Error("cache/database: sweep failed", "error", err)
			}
		}
	}
}

func (b *Backend) sweep(ctx context.Context) error {
	now := time.Now().UnixNano()

	for {
		stale, err := pack.Of[entryRow](b.db).
			Where(entryCol.ExpiresAt.Gt(0)).
			Where(entryCol.ExpiresAt.Lte(now)).
			Order(entryCol.ID.Asc()).
			Limit(int64(b.sweepBatchSize)).
			Select(entryCol.ID).
			Find(ctx)
		if err != nil {
			return err
		}
		if len(stale) == 0 {
			return nil
		}

		ids := make([]int64, len(stale))
		for i, row := range stale {
			ids[i] = row.ID
		}

		if _, err := pack.Of[entryRow](b.db).Where(entryCol.ID.In(ids...)).DeleteAll(ctx); err != nil {
			return err
		}

		if len(stale) < b.sweepBatchSize {
			return nil
		}
	}
}
