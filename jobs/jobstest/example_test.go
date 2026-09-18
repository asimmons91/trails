package jobstest_test

import (
	"context"
	"fmt"

	"github.com/asimmons91/trails/jobs"
	"github.com/asimmons91/trails/jobs/jobstest"
)

type welcomeJob struct {
	ran bool
}

func (j *welcomeJob) Kind() string { return "welcome" }

func (j *welcomeJob) Perform(ctx context.Context) error {
	j.ran = true
	fmt.Println("welcome email sent")
	return nil
}

// Example shows that Recorder.Enqueue only records a job — it doesn't run
// it the way a real jobs.Backend would — until PerformEnqueued dispatches
// everything recorded so far.
func Example() {
	reg := jobs.NewRegistry()
	job := &welcomeJob{}
	reg.Register("welcome", func() jobs.Job { return job })

	rec := jobstest.NewRecorder(reg)

	if err := jobs.Enqueue(context.Background(), rec, &welcomeJob{}); err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("ran before dispatch:", job.ran)

	if err := rec.PerformEnqueued(context.Background()); err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("ran after dispatch:", job.ran)

	// Output:
	// ran before dispatch: false
	// welcome email sent
	// ran after dispatch: true
}
