package dbqueue

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/asimmons91/trails/pack"
)

const concurrencyGuard = `(concurrency_key = '' OR (` +
	`SELECT COUNT(*) FROM dbqueue_jobs AS cc ` +
	`WHERE cc.concurrency_key = dbqueue_jobs.concurrency_key ` +
	`AND cc.state = '` + stateExecuting + `' ` +
	`AND cc.locked_at > ($1 - dbqueue_jobs.concurrency_duration_ns)` +
	`) < concurrency_limit)`

func claim(ctx context.Context, db *pack.DB, workerID string, queues []string, batchSize int, now int64) ([]jobRow, error) {
	claimOne := claimOneSQLite
	if db.Dialect().SupportsRowLocking() {
		claimOne = claimOneLocking
	}

	claimed := make([]jobRow, 0, batchSize)
	for len(claimed) < batchSize {
		row, ok, err := claimOne(ctx, db, workerID, queues, now)
		if err != nil {
			return claimed, err
		}
		if !ok {
			break
		}
		claimed = append(claimed, row)
	}

	return claimed, nil
}

func claimOneLocking(ctx context.Context, db *pack.DB, workerID string, queues []string, now int64) (jobRow, bool, error) {
	var claimed jobRow
	var ok bool

	err := db.Tx(ctx, func(tx *pack.DB) error {
		candidate, err := pack.Of[jobRow](tx).
			Where(jobCol.State.Eq(stateAvailable)).
			Where(jobCol.ScheduledAt.Lte(now)).
			Where(jobCol.Queue.In(queues...)).
			WhereRaw(concurrencyGuard, now).
			Order(jobCol.ScheduledAt.Asc()).
			ForUpdate().
			SkipLocked().
			First(ctx)
		if err != nil {
			if errors.Is(err, pack.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("dbqueue: select claim candidate: %w", err)
		}

		if _, err := pack.Of[jobRow](tx).
			Where(jobCol.ID.Eq(candidate.ID)).
			Update(ctx,
				jobCol.State.Set(stateExecuting),
				jobCol.LockedAt.Set(now),
				jobCol.LockedBy.Set(workerID),
			); err != nil {
			return fmt.Errorf("dbqueue: mark claimed row executing: %w", err)
		}

		candidate.State = stateExecuting
		candidate.LockedAt = now
		candidate.LockedBy = workerID
		claimed, ok = candidate, true
		return nil
	})

	return claimed, ok, err
}

func claimOneSQLite(ctx context.Context, db *pack.DB, workerID string, queues []string, now int64) (jobRow, bool, error) {
	d := db.Dialect()

	var args []any
	next := func(v any) string {
		args = append(args, v)
		return d.Placeholder(len(args))
	}

	setClause := fmt.Sprintf("state = %s, locked_at = %s, locked_by = %s",
		next(stateExecuting), next(now), next(workerID))

	stateArg := next(stateAvailable)
	scheduledAtArg := next(now)

	queuePlaceholders := make([]string, len(queues))
	for i, q := range queues {
		queuePlaceholders[i] = next(q)
	}

	guard := strings.ReplaceAll(concurrencyGuard, "$1", next(now))

	sqlText := fmt.Sprintf(
		"UPDATE dbqueue_jobs SET %s WHERE id = "+
			"(SELECT id FROM dbqueue_jobs WHERE state = %s AND scheduled_at <= %s "+
			"AND queue IN (%s) AND %s ORDER BY scheduled_at ASC LIMIT 1) RETURNING *",
		setClause, stateArg, scheduledAtArg, strings.Join(queuePlaceholders, ", "), guard,
	)

	rows, err := pack.Raw[jobRow](ctx, db, sqlText, args...)
	if err != nil {
		return jobRow{}, false, fmt.Errorf("dbqueue: claim row: %w", err)
	}
	if len(rows) == 0 {
		return jobRow{}, false, nil
	}

	return rows[0], true, nil
}
