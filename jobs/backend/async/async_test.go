package async_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/asimmons91/trails/jobs"
	"github.com/asimmons91/trails/jobs/backend/async"
	"github.com/stretchr/testify/require"
)

type countingJob struct {
	count *atomic.Int32
}

func (j *countingJob) Kind() string { return "counting" }

func (j *countingJob) Perform(ctx context.Context) error {
	j.count.Add(1)
	return nil
}

func TestBackendRunsEnqueuedJobsConcurrently(t *testing.T) {
	reg := jobs.NewRegistry()
	var ran atomic.Int32
	reg.Register("counting", func() jobs.Job { return &countingJob{count: &ran} })

	b := async.New(reg, 4)
	defer b.Close()

	for range 10 {
		require.NoError(t, jobs.Enqueue(context.Background(), b, &countingJob{count: &ran}))
	}

	require.NoError(t, b.Drain(context.Background()))
	require.EqualValues(t, 10, ran.Load())
}

type recordingStartJob struct {
	started chan time.Time
}

func (j *recordingStartJob) Kind() string { return "recording-start" }

func (j *recordingStartJob) Perform(ctx context.Context) error {
	j.started <- time.Now()
	return nil
}

func TestBackendHonorsScheduledAtDelay(t *testing.T) {
	reg := jobs.NewRegistry()
	started := make(chan time.Time, 1)
	reg.Register("recording-start", func() jobs.Job { return &recordingStartJob{started: started} })

	b := async.New(reg, 1)
	defer b.Close()

	enqueuedAt := time.Now()
	require.NoError(t, jobs.Enqueue(context.Background(), b, &recordingStartJob{started: started}, jobs.WithDelay(150*time.Millisecond)))

	select {
	case startedAt := <-started:
		require.GreaterOrEqual(t, startedAt.Sub(enqueuedAt), 100*time.Millisecond)
	case <-time.After(2 * time.Second):
		t.Fatal("job never ran")
	}
}

type concurrencyTrackingJob struct {
	current *atomic.Int32
	max     *atomic.Int32
}

func (j *concurrencyTrackingJob) Kind() string { return "concurrency-tracking" }

func (j *concurrencyTrackingJob) ConcurrencyKey() string { return "shared" }

func (j *concurrencyTrackingJob) ConcurrencyLimit() (int, time.Duration) { return 1, time.Minute }

func (j *concurrencyTrackingJob) Perform(ctx context.Context) error {
	now := j.current.Add(1)
	for {
		max := j.max.Load()
		if now <= max || j.max.CompareAndSwap(max, now) {
			break
		}
	}
	time.Sleep(30 * time.Millisecond)
	j.current.Add(-1)
	return nil
}

func TestBackendEnforcesConcurrencyLimit(t *testing.T) {
	reg := jobs.NewRegistry()
	var current, max atomic.Int32
	reg.Register("concurrency-tracking", func() jobs.Job {
		return &concurrencyTrackingJob{current: &current, max: &max}
	})

	b := async.New(reg, 4)
	defer b.Close()

	for range 5 {
		require.NoError(t, jobs.Enqueue(context.Background(), b, &concurrencyTrackingJob{current: &current, max: &max}))
	}

	require.NoError(t, b.Drain(context.Background()))
	require.EqualValues(t, 1, max.Load(), "no more than one job sharing the concurrency key should run at once")
}

func TestBackendCloseWaitsForWorkersToStop(t *testing.T) {
	b := async.New(jobs.NewRegistry(), 2)
	require.NoError(t, b.Close())
}

func TestBackendDrainReturnsImmediatelyWhenIdle(t *testing.T) {
	b := async.New(jobs.NewRegistry(), 1)
	defer b.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, b.Drain(ctx))
}
