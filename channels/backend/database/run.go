package database

import (
	"context"
	"errors"
	"time"

	"github.com/asimmons91/trails/pack"
)

// Run polls for and delivers new messages, and periodically trims
// expired ones, until ctx is cancelled. It satisfies trails.Runner, and
// must be kept running (e.g. via RegisterSpurRunners) for this Backend to
// ever deliver anything — Publish only writes rows; Run is what turns
// them into delivered messages, locally and in every other process
// polling the same table. A subscriber slow enough to fall behind
// delivery has messages dropped rather than blocking the poll loop for
// everyone else.
func (b *Backend) Run(ctx context.Context) error {
	pollTicker := time.NewTicker(b.pollInterval)
	defer pollTicker.Stop()

	trimTicker := time.NewTicker(b.trimInterval)
	defer trimTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-pollTicker.C:
			if err := b.poll(ctx); err != nil && !errors.Is(err, context.Canceled) {
				b.logger.Error("database: poll failed", "error", err)
			}
		case <-trimTicker.C:
			if err := b.trim(ctx); err != nil && !errors.Is(err, context.Canceled) {
				b.logger.Error("database: trim failed", "error", err)
			}
		}
	}
}

func (b *Backend) poll(ctx context.Context) error {
	for {
		rows, err := pack.Of[messageRow](b.db).
			Where(messageCol.ID.Gt(b.cursor)).
			Order(messageCol.ID.Asc()).
			Limit(int64(b.batchSize)).
			Find(ctx)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}

		b.deliver(rows)
		b.cursor = rows[len(rows)-1].ID

		if len(rows) < b.batchSize {
			return nil
		}
	}
}

func (b *Backend) deliver(rows []messageRow) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, row := range rows {
		payload := []byte(row.Payload)
		for sub := range b.subs[row.Topic] {
			select {
			case sub.messages <- payload:
			default:
				// slow subscriber: drop rather than block the poll loop
			}
		}
	}
}

func (b *Backend) trim(ctx context.Context) error {
	cutoff := time.Now().Add(-b.retention).UnixNano()

	stale, err := pack.Of[messageRow](b.db).
		Where(messageCol.CreatedAt.Lt(cutoff)).
		Order(messageCol.CreatedAt.Asc()).
		Limit(int64(b.trimBatchSize)).
		Select(messageCol.ID).
		Find(ctx)
	if err != nil {
		return err
	}
	if len(stale) == 0 {
		return nil
	}

	ids := make([]int64, len(stale))
	for i, r := range stale {
		ids[i] = r.ID
	}

	_, err = pack.Of[messageRow](b.db).Where(messageCol.ID.In(ids...)).DeleteAll(ctx)
	return err
}
