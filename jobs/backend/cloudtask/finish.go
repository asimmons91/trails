package cloudtask

import (
	"context"
	"time"

	"github.com/asimmons91/trails/pack"
)

func (b *Backend) finish(ctx context.Context, id int64) error {
	_, err := pack.Of[jobRow](b.db).Where(jobCol.ID.Eq(id)).Update(ctx,
		jobCol.State.Set(stateFinished),
		jobCol.FinishedAt.Set(time.Now().UnixNano()),
	)
	return err
}

func (b *Backend) fail(ctx context.Context, row jobRow, jobErr error) (exhausted bool, err error) {
	exhausted = row.Attempts >= row.MaxAttempts

	assignments := []pack.Assignment{jobCol.LastError.Set(jobErr.Error())}
	if exhausted {
		// FinishedAt must be stamped here too, not just on success: cleanupJobs
		// filters on FinishedAt <= cutoff, and a zero-value FinishedAt (0) is
		// always <= a positive cutoff, which would otherwise make every failed
		// row eligible for deletion immediately instead of after retention.
		assignments = append(assignments,
			jobCol.State.Set(stateFailed),
			jobCol.FinishedAt.Set(time.Now().UnixNano()),
		)
	} else {
		assignments = append(assignments, jobCol.State.Set(statePending))
	}

	_, err = pack.Of[jobRow](b.db).Where(jobCol.ID.Eq(row.ID)).Update(ctx, assignments...)
	return exhausted, err
}
