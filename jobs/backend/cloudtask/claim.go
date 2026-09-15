package cloudtask

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/asimmons91/trails/pack"
)

type claimResult int

const (
	claimExecute   claimResult = iota // proceed to Dispatch
	claimSkip                         // not pending (already executing/finished/failed, or the row no longer exists) — ack, no-op
	claimDeferred                     // concurrency limit currently full — Cloud Tasks should retry
	claimExhausted                    // attempts already at/over max — ack, no-op, permanent failure
)

func (b *Backend) claimForExecution(ctx context.Context, jobID int64) (jobRow, claimResult, error) {
	var row jobRow
	result := claimSkip
	now := time.Now().UnixNano()

	err := b.db.Tx(ctx, func(tx *pack.DB) error {
		q := pack.Of[jobRow](tx).Where(jobCol.ID.Eq(jobID))
		if tx.Dialect().SupportsRowLocking() {
			q = q.ForUpdate()
		}

		var err error
		row, err = q.First(ctx)
		if err != nil {
			if errors.Is(err, pack.ErrNoRows) {
				return nil // claimSkip: task references a row that no longer exists
			}
			return fmt.Errorf("cloudtask: lock job row: %w", err)
		}

		if row.State != statePending {
			return nil // claimSkip: duplicate/late delivery of an already-handled task
		}
		if row.Attempts >= row.MaxAttempts {
			result = claimExhausted
			return nil
		}

		row.Attempts++
		row.State = stateExecuting
		row.LockedAt = now
		if _, err := pack.Of[jobRow](tx).Where(jobCol.ID.Eq(row.ID)).Update(ctx,
			jobCol.Attempts.Set(row.Attempts),
			jobCol.State.Set(stateExecuting),
			jobCol.LockedAt.Set(now),
		); err != nil {
			return fmt.Errorf("cloudtask: mark claimed row executing: %w", err)
		}

		if row.ConcurrencyKey == "" {
			result = claimExecute
			return nil
		}

		if err := pack.Create(ctx, tx, &slotRow{ConcurrencyKey: row.ConcurrencyKey, CreatedAt: now},
			pack.OnConflict[slotRow](slotCol.ConcurrencyKey).DoNothing()); err != nil {
			return fmt.Errorf("cloudtask: upsert concurrency slot: %w", err)
		}

		slotQ := pack.Of[slotRow](tx).Where(slotCol.ConcurrencyKey.Eq(row.ConcurrencyKey))
		if tx.Dialect().SupportsRowLocking() {
			slotQ = slotQ.ForUpdate()
		}
		if _, err := slotQ.First(ctx); err != nil {
			return fmt.Errorf("cloudtask: lock concurrency slot: %w", err)
		}

		windowStart := now - row.ConcurrencyDurationNs
		count, err := pack.Of[jobRow](tx).
			Where(jobCol.ConcurrencyKey.Eq(row.ConcurrencyKey)).
			Where(jobCol.State.Eq(stateExecuting)).
			Where(jobCol.LockedAt.Gt(windowStart)).
			Count(ctx)
		if err != nil {
			return fmt.Errorf("cloudtask: count active for concurrency key: %w", err)
		}

		if count > int64(row.ConcurrencyLimit) {
			// Over budget (count includes the row just claimed above) —
			// release it and undo the attempt bump: a deferral due to our
			// own throttling isn't a job failure and shouldn't consume
			// MaxAttempts.
			row.Attempts--
			row.State = statePending
			if _, err := pack.Of[jobRow](tx).Where(jobCol.ID.Eq(row.ID)).Update(ctx,
				jobCol.Attempts.Set(row.Attempts),
				jobCol.State.Set(statePending),
			); err != nil {
				return fmt.Errorf("cloudtask: release deferred claim: %w", err)
			}
			result = claimDeferred
			return nil
		}

		result = claimExecute
		return nil
	})

	return row, result, err
}
