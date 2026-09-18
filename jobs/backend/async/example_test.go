package async_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/asimmons91/trails/jobs"
	"github.com/asimmons91/trails/jobs/backend/async"
)

type limitedJob struct {
	current, max *atomic.Int32
}

func (j *limitedJob) Kind() string { return "limited" }

func (j *limitedJob) ConcurrencyKey() string { return "shared-key" }

func (j *limitedJob) ConcurrencyLimit() (int, time.Duration) { return 1, time.Minute }

func (j *limitedJob) Perform(ctx context.Context) error {
	n := j.current.Add(1)
	defer j.current.Add(-1)

	for {
		m := j.max.Load()
		if n <= m || j.max.CompareAndSwap(m, n) {
			break
		}
	}

	return nil
}

// Example shows Backend enforcing a job's ConcurrencyLimit: several jobs
// sharing the same ConcurrencyKey are enqueued at once, but Backend runs at
// most one of them at a time, re-queueing the rest until a slot frees up.
func Example() {
	reg := jobs.NewRegistry()
	var current, max atomic.Int32
	reg.Register("limited", func() jobs.Job { return &limitedJob{current: &current, max: &max} })

	b := async.New(reg, 4)
	defer b.Close()

	for range 5 {
		if err := jobs.Enqueue(context.Background(), b, &limitedJob{current: &current, max: &max}); err != nil {
			fmt.Println("error:", err)
			return
		}
	}

	if err := b.Drain(context.Background()); err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println("max concurrent:", max.Load())
	// Output:
	// max concurrent: 1
}
