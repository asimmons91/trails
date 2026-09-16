package sqlite_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/asimmons91/trails/jobs"
	"github.com/asimmons91/trails/jobs/backend/dbqueue"
	"github.com/asimmons91/trails/pack"
	"github.com/asimmons91/trails/pack/driver/sqlite"
	"github.com/asimmons91/trails/pack/migrate"
	"github.com/stretchr/testify/require"
)

// own query API without dbqueue needing to export any of it.
type jobRowView struct {
	pack.Model[int64] `db:"table:dbqueue_jobs"`
	State             string `db:"state"`
	Attempts          int    `db:"attempts"`
	FinishedAt        int64  `db:"finished_at"`
}

func newTestDB(t *testing.T) *pack.DB {
	t.Helper()

	d := sqlite.New()
	sqlDB, err := d.Open(":memory:")
	require.NoError(t, err)
	// dbqueue polls and dispatches concurrently; a fresh connection per
	// query against ":memory:" would each see an independent empty
	// database, so pin the pool to the one connection that ran migrations.
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	db := pack.Open(sqlDB, d.Dialect())
	require.NoError(t, dbqueue.Migration.Migrate(context.Background(), migrate.New(db)))
	return db
}

// runBackend starts b.Run in the background and stops it (waiting for
// in-flight dispatches) on test cleanup.
func runBackend(t *testing.T, b *dbqueue.Backend) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- b.Run(ctx) }()

	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("dbqueue.Backend.Run did not stop after context cancellation")
		}
	})
}

type countingJob struct {
	count *atomic.Int32
}

func (countingJob) Kind() string { return "counting" }

func (j *countingJob) Perform(context.Context) error {
	j.count.Add(1)
	return nil
}

func TestDBQueue_EnqueueAndRun_ExecutesJob(t *testing.T) {
	db := newTestDB(t)
	reg := jobs.NewRegistry()
	var ran atomic.Int32
	reg.Register("counting", func() jobs.Job { return &countingJob{count: &ran} })

	b := dbqueue.New(db, reg, dbqueue.WithPollInterval(20*time.Millisecond))
	runBackend(t, b)

	require.NoError(t, jobs.Enqueue(context.Background(), b, &countingJob{count: &ran}))

	require.Eventually(t, func() bool { return ran.Load() == 1 }, 3*time.Second, 10*time.Millisecond)
}

type recordingStartJob struct {
	started chan time.Time
}

func (recordingStartJob) Kind() string { return "recording-start" }

func (j *recordingStartJob) Perform(context.Context) error {
	j.started <- time.Now()
	return nil
}

func TestDBQueue_HonorsScheduledDelay(t *testing.T) {
	db := newTestDB(t)
	reg := jobs.NewRegistry()
	started := make(chan time.Time, 1)
	reg.Register("recording-start", func() jobs.Job { return &recordingStartJob{started: started} })

	b := dbqueue.New(db, reg, dbqueue.WithPollInterval(20*time.Millisecond))
	runBackend(t, b)

	enqueuedAt := time.Now()
	require.NoError(t, jobs.Enqueue(context.Background(), b, &recordingStartJob{started: started}, jobs.WithDelay(200*time.Millisecond)))

	select {
	case startedAt := <-started:
		require.GreaterOrEqual(t, startedAt.Sub(enqueuedAt), 150*time.Millisecond)
	case <-time.After(3 * time.Second):
		t.Fatal("job never ran")
	}
}

type flakyJob struct {
	failures int
	attempts *atomic.Int32
	done     chan struct{}
}

func (flakyJob) Kind() string { return "flaky" }

func (j *flakyJob) Perform(context.Context) error {
	n := j.attempts.Add(1)
	if int(n) <= j.failures {
		return errors.New("synthetic failure")
	}
	close(j.done)
	return nil
}

func TestDBQueue_RetriesOnFailure_ThenSucceeds(t *testing.T) {
	db := newTestDB(t)
	reg := jobs.NewRegistry()
	var attempts atomic.Int32
	done := make(chan struct{})
	reg.Register("flaky", func() jobs.Job { return &flakyJob{failures: 2, attempts: &attempts, done: done} })

	b := dbqueue.New(db, reg,
		dbqueue.WithPollInterval(10*time.Millisecond),
		dbqueue.WithBackoff(5*time.Millisecond, 20*time.Millisecond),
	)
	runBackend(t, b)

	require.NoError(t, jobs.Enqueue(context.Background(), b, &flakyJob{failures: 2, attempts: &attempts, done: done}, jobs.WithMaxAttempts(5)))

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("job never succeeded")
	}
	require.EqualValues(t, 3, attempts.Load())
}

type alwaysFailJob struct {
	attempts *atomic.Int32
}

func (alwaysFailJob) Kind() string { return "always-fail" }

func (j *alwaysFailJob) Perform(context.Context) error {
	j.attempts.Add(1)
	return errors.New("boom")
}

func TestDBQueue_MaxAttemptsExhausted_StopsRetrying(t *testing.T) {
	db := newTestDB(t)
	reg := jobs.NewRegistry()
	var attempts atomic.Int32
	reg.Register("always-fail", func() jobs.Job { return &alwaysFailJob{attempts: &attempts} })

	b := dbqueue.New(db, reg,
		dbqueue.WithPollInterval(10*time.Millisecond),
		dbqueue.WithBackoff(5*time.Millisecond, 20*time.Millisecond),
	)
	runBackend(t, b)

	require.NoError(t, jobs.Enqueue(context.Background(), b, &alwaysFailJob{attempts: &attempts}, jobs.WithMaxAttempts(3)))

	require.Eventually(t, func() bool { return attempts.Load() == 3 }, 5*time.Second, 10*time.Millisecond)

	time.Sleep(150 * time.Millisecond) // long enough for another retry if the cap were broken
	require.EqualValues(t, 3, attempts.Load(), "should not retry past MaxAttempts")

	rows, err := pack.Of[jobRowView](db).Find(context.Background())
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "failed", rows[0].State)
}

type concurrencyJob struct {
	current, max *atomic.Int32
	release      <-chan struct{}
}

func (concurrencyJob) Kind() string { return "concurrency" }

func (concurrencyJob) ConcurrencyKey() string { return "shared" }

func (concurrencyJob) ConcurrencyLimit() (int, time.Duration) { return 1, time.Minute }

func (j *concurrencyJob) Perform(context.Context) error {
	now := j.current.Add(1)
	for {
		m := j.max.Load()
		if now <= m || j.max.CompareAndSwap(m, now) {
			break
		}
	}
	<-j.release
	j.current.Add(-1)
	return nil
}

func TestDBQueue_EnforcesConcurrencyLimit(t *testing.T) {
	db := newTestDB(t)
	reg := jobs.NewRegistry()
	var current, max atomic.Int32
	release := make(chan struct{})
	reg.Register("concurrency", func() jobs.Job {
		return &concurrencyJob{current: &current, max: &max, release: release}
	})

	b := dbqueue.New(db, reg,
		dbqueue.WithPollInterval(10*time.Millisecond),
		dbqueue.WithBatchSize(5),
		dbqueue.WithWorkers(5),
	)
	runBackend(t, b)

	for range 3 {
		require.NoError(t, jobs.Enqueue(context.Background(), b, &concurrencyJob{current: &current, max: &max, release: release}))
	}

	// Give every job a chance to be claimed and start Perform before
	// releasing any of them, so an over-claim would show up in max.
	time.Sleep(300 * time.Millisecond)
	close(release)

	require.Eventually(t, func() bool { return current.Load() == 0 }, 3*time.Second, 10*time.Millisecond)
	require.EqualValues(t, 1, max.Load(), "no more than one job sharing the concurrency key should run at once")
}

func TestDBQueue_CleanupSweepsOldFinishedRows(t *testing.T) {
	db := newTestDB(t)
	reg := jobs.NewRegistry()
	var ran atomic.Int32
	reg.Register("counting", func() jobs.Job { return &countingJob{count: &ran} })

	b := dbqueue.New(db, reg,
		dbqueue.WithPollInterval(10*time.Millisecond),
		dbqueue.WithRetention(50*time.Millisecond),
		dbqueue.WithCleanupInterval(20*time.Millisecond),
	)
	runBackend(t, b)

	require.NoError(t, jobs.Enqueue(context.Background(), b, &countingJob{count: &ran}))
	require.Eventually(t, func() bool { return ran.Load() == 1 }, 3*time.Second, 10*time.Millisecond)

	require.Eventually(t, func() bool {
		n, err := pack.Of[jobRowView](db).Count(context.Background())
		require.NoError(t, err)
		return n == 0
	}, 3*time.Second, 10*time.Millisecond, "finished row should be swept once past retention")
}
