package cloudtask

import (
	"context"
	"net/http"
	"time"

	trails "github.com/asimmons91/trails"
	"github.com/asimmons91/trails/pack"
)

const defaultCleanupBatchSize = 500

func (b *Backend) handleCleanupPush(c *trails.Context) error {
	ctx := c.Request().Context()

	if err := b.authenticate(ctx, c.Request()); err != nil {
		return c.String(http.StatusUnauthorized, "unauthorized")
	}

	var body cleanupPayload
	if err := c.Bind(&body); err != nil {
		return c.String(http.StatusBadRequest, "bad request")
	}

	claimed := false
	err := b.db.Tx(ctx, func(tx *pack.DB) error {
		q := pack.Of[scheduleRow](tx).Where(scheduleCol.Name.Eq(scheduleNameCleanup))
		if tx.Dialect().SupportsRowLocking() {
			q = q.ForUpdate()
		}
		row, err := q.First(ctx)
		if err != nil {
			return err
		}

		if row.NextRunAt != body.ScheduledFor {
			return nil // duplicate delivery, or superseded by a concurrent bootstrap
		}

		_, err = pack.Of[scheduleRow](tx).Where(scheduleCol.ID.Eq(row.ID)).Update(ctx,
			scheduleCol.NextRunAt.Set(scheduleClaimedSentinel),
		)
		if err != nil {
			return err
		}
		claimed = true
		return nil
	})
	if err != nil {
		c.Logger().Error("cloudtask: claim cleanup push", "error", err)
		return c.String(http.StatusInternalServerError, "internal error")
	}
	if !claimed {
		return c.String(http.StatusOK, "ok")
	}

	if err := b.cleanupJobs(ctx); err != nil {
		c.Logger().Error("cloudtask: cleanup jobs failed", "error", err)
	}
	if err := b.cleanupSlots(ctx); err != nil {
		c.Logger().Error("cloudtask: cleanup slots failed", "error", err)
	}

	next := time.Now().Add(b.cleanupInterval).UnixNano()
	if err := b.scheduleCleanupTask(ctx, next); err != nil {
		c.Logger().Error("cloudtask: reschedule cleanup task", "error", err)
		// Don't leave NextRunAt at the sentinel forever: reset to 0 so the
		// chain is picked back up by the next Enqueue-driven ensureScheduled
		// call. A Cloud Tasks retry of this same push won't help — its
		// scheduled_for would no longer match once this row changes again,
		// so it would just hit the no-op branch above.
		if _, resetErr := pack.Of[scheduleRow](b.db).Where(scheduleCol.Name.Eq(scheduleNameCleanup)).Update(ctx,
			scheduleCol.NextRunAt.Set(int64(0)),
		); resetErr != nil {
			c.Logger().Error("cloudtask: reset cleanup schedule after failed reschedule", "error", resetErr)
		}
		return c.String(http.StatusOK, "ok")
	}

	if _, err := pack.Of[scheduleRow](b.db).Where(scheduleCol.Name.Eq(scheduleNameCleanup)).Update(ctx,
		scheduleCol.NextRunAt.Set(next),
	); err != nil {
		c.Logger().Error("cloudtask: record next cleanup schedule", "error", err)
	}

	return c.String(http.StatusOK, "ok")
}

// cleanupJobs deletes finished/failed cloudtask_jobs rows older than
// retention, in batches, mirroring dbqueue's cleanup.
func (b *Backend) cleanupJobs(ctx context.Context) error {
	cutoff := time.Now().Add(-b.retention).UnixNano()

	stale, err := pack.Of[jobRow](b.db).
		Where(jobCol.State.In(stateFinished, stateFailed)).
		Where(jobCol.FinishedAt.Lte(cutoff)).
		Order(jobCol.FinishedAt.Asc()).
		Limit(defaultCleanupBatchSize).
		Select(jobCol.ID).
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

	_, err = pack.Of[jobRow](b.db).Where(jobCol.ID.In(ids...)).DeleteAll(ctx)
	return err
}

// cleanupSlots deletes cloudtask_concurrency_slots rows older than
// slotGrace with no matching executing job. Deliberately unsynchronized
// with claimForExecution: worst case a slot is deleted and immediately
// re-created by a fresh claim via ON CONFLICT DO NOTHING, which is harmless
// churn, not a correctness issue.
func (b *Backend) cleanupSlots(ctx context.Context) error {
	cutoff := time.Now().Add(-b.slotGrace).UnixNano()

	stale, err := pack.Of[slotRow](b.db).
		Where(slotCol.CreatedAt.Lte(cutoff)).
		Order(slotCol.CreatedAt.Asc()).
		Limit(defaultCleanupBatchSize).
		Find(ctx)
	if err != nil {
		return err
	}
	if len(stale) == 0 {
		return nil
	}

	for _, s := range stale {
		active, err := pack.Of[jobRow](b.db).
			Where(jobCol.ConcurrencyKey.Eq(s.ConcurrencyKey)).
			Where(jobCol.State.Eq(stateExecuting)).
			Exists(ctx)
		if err != nil {
			return err
		}
		if active {
			continue
		}

		if _, err := pack.Of[slotRow](b.db).Where(slotCol.ID.Eq(s.ID)).Delete(ctx); err != nil {
			return err
		}
	}

	return nil
}
