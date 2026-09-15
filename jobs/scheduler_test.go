package jobs

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

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

func TestSchedulerEnqueuesDueSchedulesOnEachTick(t *testing.T) {
	reg := NewRegistry()
	var runs atomic.Int32
	reg.Register("counting", func() Job { return &countingJob{count: &runs} })
	reg.RegisterSchedule("tick-every-tick", 0, func() Job { return &countingJob{count: &runs} })

	b := &fakeBackend{}
	sched := NewScheduler(reg, b)
	sched.nextRun = map[string]time.Time{} // ensure fresh, first tick fires immediately

	sched.tick(context.Background(), time.Now())
	require.Len(t, b.enqueued, 1)
	require.Equal(t, "counting", b.enqueued[0].Kind)
}

func TestSchedulerSkipsScheduleNotYetDue(t *testing.T) {
	reg := NewRegistry()
	var runs atomic.Int32
	reg.Register("counting", func() Job { return &countingJob{count: &runs} })
	reg.RegisterSchedule("hourly", time.Hour, func() Job { return &countingJob{count: &runs} })

	b := &fakeBackend{}
	sched := NewScheduler(reg, b)

	now := time.Now()
	sched.tick(context.Background(), now)
	require.Len(t, b.enqueued, 1, "first tick should fire immediately")

	sched.tick(context.Background(), now.Add(time.Minute))
	require.Len(t, b.enqueued, 1, "second tick within the hour should not re-fire")

	sched.tick(context.Background(), now.Add(time.Hour+time.Second))
	require.Len(t, b.enqueued, 2, "tick past the interval should fire again")
}

func TestSchedulerRunStopsOnContextCancel(t *testing.T) {
	reg := NewRegistry()
	b := &fakeBackend{}
	sched := NewScheduler(reg, b)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- sched.Run(ctx) }()

	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop after context cancellation")
	}
}
