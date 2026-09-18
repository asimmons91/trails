package jobs_test

import (
	"context"
	"fmt"

	"github.com/asimmons91/trails/jobs"
	"github.com/asimmons91/trails/jobs/jobstest"
)

type greetJob struct {
	Name string
}

func (j *greetJob) Kind() string { return "greet" }

func (j *greetJob) Perform(ctx context.Context) error {
	fmt.Println("hello,", j.Name)
	return nil
}

// Example shows the core jobs lifecycle: define a Job, register its kind
// with a Registry, Enqueue it against a Backend, then have the Backend's
// recorded work Dispatched back through the Registry. jobstest.Recorder
// stands in for a real Backend here.
func Example() {
	reg := jobs.NewRegistry()
	reg.Register("greet", func() jobs.Job { return &greetJob{} })

	rec := jobstest.NewRecorder(reg)

	if err := jobs.Enqueue(context.Background(), rec, &greetJob{Name: "Ada"}); err != nil {
		fmt.Println("error:", err)
		return
	}

	if err := rec.PerformEnqueued(context.Background()); err != nil {
		fmt.Println("error:", err)
		return
	}

	// Output:
	// hello, Ada
}
