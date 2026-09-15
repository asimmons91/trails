package mysql_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/asimmons91/trails/driver/mysql"
	"github.com/asimmons91/trails/jobs"
	"github.com/asimmons91/trails/jobs/backend/dbqueue"
	"github.com/asimmons91/trails/pack"
	"github.com/asimmons91/trails/pack/migrate"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
)

func newDBQueueTestDB(t *testing.T) *pack.DB {
	t.Helper()
	ctx := context.Background()

	mc, err := tcmysql.Run(ctx,
		"mysql:8",
		tcmysql.WithDatabase("pack"),
		tcmysql.WithUsername("pack"),
		tcmysql.WithPassword("pack"),
	)
	testcontainers.CleanupContainer(t, mc)
	require.NoError(t, err)

	connStr, err := mc.ConnectionString(ctx)
	require.NoError(t, err)

	db, err := pack.Connect(mysql.New(), connStr)
	require.NoError(t, err)

	require.NoError(t, dbqueue.Migration.Migrate(ctx, migrate.New(db)))
	return db
}

// markerJob records, per distinct N, how many times it was Performed. Used
// to detect double-claims: under a correct SKIP LOCKED claim, every enqueued
// job runs exactly once even with many workers claiming concurrently.
type markerJob struct {
	N    int       `json:"n"`
	seen *sync.Map // int -> *atomic.Int32, set by the registered factory
}

func (markerJob) Kind() string { return "marker" }

func (j *markerJob) Perform(context.Context) error {
	v, _ := j.seen.LoadOrStore(j.N, new(atomic.Int32))
	v.(*atomic.Int32).Add(1)
	return nil
}

// TestDBQueue_SkipLockedClaimIsExclusiveAcrossConcurrentWorkers enqueues a
// batch of jobs and runs many dbqueue.Backend "worker processes" against the
// same MySQL database concurrently. MySQL supports row locking, so this
// exercises the FOR UPDATE SKIP LOCKED claim path (claimOneLocking) — the
// property SQLite's single-writer test can't prove, since SQLite never has
// two real concurrent claimers.
func TestDBQueue_SkipLockedClaimIsExclusiveAcrossConcurrentWorkers(t *testing.T) {
	db := newDBQueueTestDB(t)

	const numJobs = 40
	const numWorkers = 8

	reg := jobs.NewRegistry()
	var seen sync.Map
	reg.Register("marker", func() jobs.Job { return &markerJob{seen: &seen} })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	for range numWorkers {
		b := dbqueue.New(db, reg,
			dbqueue.WithPollInterval(5*time.Millisecond),
			dbqueue.WithBatchSize(2),
			dbqueue.WithWorkers(2),
		)
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = b.Run(ctx)
		}()
	}

	enqueuer := dbqueue.New(db, reg)
	for i := range numJobs {
		require.NoError(t, jobs.Enqueue(context.Background(), enqueuer, &markerJob{N: i, seen: &seen}))
	}

	require.Eventually(t, func() bool {
		count := 0
		seen.Range(func(_, _ any) bool { count++; return true })
		return count == numJobs
	}, 20*time.Second, 25*time.Millisecond, "every job should eventually run")

	// Give any would-be duplicate claim time to land, then confirm none did.
	time.Sleep(500 * time.Millisecond)

	total := 0
	seen.Range(func(key, v any) bool {
		n := v.(*atomic.Int32).Load()
		require.EqualValuesf(t, 1, n, "job %v ran %d times, want exactly 1 (double-claim)", key, n)
		total++
		return true
	})
	require.Equal(t, numJobs, total)

	cancel()
	wg.Wait()
}
