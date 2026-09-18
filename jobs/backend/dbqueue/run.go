package dbqueue

import (
	"context"
	"encoding/json"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/asimmons91/trails/jobs"
	"github.com/asimmons91/trails/pack"
)

// Run polls for available rows every WithPollInterval, dispatching up to
// WithWorkers of them concurrently through the Registry, and periodically
// deletes rows past WithRetention. It's what actually processes jobs
// persisted by Enqueue — nothing runs until Run is started. It blocks until
// ctx is canceled, waits for in-flight dispatches to finish, then returns
// nil. Satisfies trails.Runner.
func (b *Backend) Run(ctx context.Context) error {
	pollTicker := time.NewTicker(b.pollInterval)
	defer pollTicker.Stop()

	cleanupTicker := time.NewTicker(b.cleanupInterval)
	defer cleanupTicker.Stop()

	sem := make(chan struct{}, b.workers)
	var wg sync.WaitGroup

	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return nil
		case <-pollTicker.C:
			b.poll(ctx, sem, &wg)
		case <-cleanupTicker.C:
			if err := b.cleanup(ctx); err != nil {
				b.logger.Error("dbqueue: cleanup failed", "error", err)
			}
		}
	}
}

func (b *Backend) poll(ctx context.Context, sem chan struct{}, wg *sync.WaitGroup) {
	rows, err := claim(ctx, b.db, b.workerID, b.queues, b.batchSize, time.Now().UnixNano())
	if err != nil {
		b.logger.Error("dbqueue: claim failed", "error", err)
		return
	}

	for _, row := range rows {
		wg.Add(1)
		sem <- struct{}{}
		go func(row jobRow) {
			defer wg.Done()
			defer func() { <-sem }()
			b.dispatch(row)
		}(row)
	}
}

func (b *Backend) dispatch(row jobRow) {
	ctx := context.Background()

	err := b.reg.Dispatch(ctx, jobs.Enqueued{
		Kind: row.Kind,
		Args: json.RawMessage(row.Args),
	})

	now := time.Now()
	if err == nil {
		b.finish(ctx, row, now)
		return
	}

	b.logger.Error("dbqueue: job failed", "id", row.ID, "kind", row.Kind, "error", err)
	b.fail(ctx, row, err, now)
}

func (b *Backend) finish(ctx context.Context, row jobRow, now time.Time) {
	_, err := pack.Of[jobRow](b.db).
		Where(jobCol.ID.Eq(row.ID)).
		Update(ctx,
			jobCol.State.Set(stateFinished),
			jobCol.FinishedAt.Set(now.UnixNano()),
		)
	if err != nil {
		b.logger.Error("dbqueue: mark job finished", "id", row.ID, "kind", row.Kind, "error", err)
	}
}

func (b *Backend) fail(ctx context.Context, row jobRow, jobErr error, now time.Time) {
	attempts := row.Attempts + 1
	assignments := []pack.Assignment{
		jobCol.Attempts.Set(attempts),
		jobCol.LastError.Set(jobErr.Error()),
	}

	if attempts >= row.MaxAttempts {
		assignments = append(assignments, jobCol.State.Set(stateFailed))
	} else {
		assignments = append(assignments,
			jobCol.State.Set(stateAvailable),
			jobCol.ScheduledAt.Set(now.Add(b.computeBackoff(attempts)).UnixNano()),
		)
	}

	if _, err := pack.Of[jobRow](b.db).Where(jobCol.ID.Eq(row.ID)).Update(ctx, assignments...); err != nil {
		b.logger.Error("dbqueue: record job failure", "id", row.ID, "kind", row.Kind, "error", err)
	}
}

// computeBackoff returns an exponentially growing, jittered delay before
// retrying a job that failed on its attempts-th try: backoffBase * 2^n,
// capped at backoffMax, with up to 50% jitter to avoid many jobs that
// failed together retrying together.
func (b *Backend) computeBackoff(attempts int) time.Duration {
	exp := attempts
	if exp > 10 {
		exp = 10
	}

	d := b.backoffBase << exp
	if d <= 0 || d > b.backoffMax {
		d = b.backoffMax
	}

	half := d / 2
	return half + time.Duration(rand.Int64N(int64(half)+1))
}

// cleanup deletes finished rows older than the retention window, in
// batches, mirroring SolidQueue's clear_finished_in_batches.
func (b *Backend) cleanup(ctx context.Context) error {
	cutoff := time.Now().Add(-b.retention).UnixNano()

	stale, err := pack.Of[jobRow](b.db).
		Where(jobCol.State.Eq(stateFinished)).
		Where(jobCol.FinishedAt.Lte(cutoff)).
		Order(jobCol.FinishedAt.Asc()).
		Limit(int64(b.cleanupBatchSize)).
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
