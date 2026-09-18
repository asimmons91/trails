package sqlite_test

import (
	"context"
	"fmt"
	"time"

	"github.com/asimmons91/trails/jobs"
	"github.com/asimmons91/trails/jobs/backend/dbqueue"
	"github.com/asimmons91/trails/pack"
	"github.com/asimmons91/trails/pack/driver/sqlite"
	"github.com/asimmons91/trails/pack/migrate"
)

// greetJob's done channel is unexported, so Enqueue's json.Marshal simply
// skips it and Registry.Dispatch's json.Unmarshal leaves it untouched —
// only the factory-constructed instance below needs one.
type greetJob struct {
	Name string
	done chan struct{}
}

func (j *greetJob) Kind() string { return "greet" }

func (j *greetJob) Perform(ctx context.Context) error {
	fmt.Println("hello,", j.Name)
	close(j.done)
	return nil
}

// Example shows a Driver backing the full dbqueue lifecycle: apply
// Migration, start Run (it does nothing until then), Enqueue a job, and let
// Run poll, claim, and dispatch it. Waiting on a channel the job closes —
// rather than sleeping — keeps the example deterministic despite Run's poll
// loop being asynchronous.
func Example() {
	driver := sqlite.New()
	sqlDB, err := driver.Open(":memory:")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	defer sqlDB.Close()

	db := pack.Open(sqlDB, driver.Dialect())
	if err := dbqueue.Migration.Migrate(context.Background(), migrate.New(db)); err != nil {
		fmt.Println("error:", err)
		return
	}

	done := make(chan struct{})
	reg := jobs.NewRegistry()
	reg.Register("greet", func() jobs.Job { return &greetJob{done: done} })

	b := dbqueue.New(db, reg, dbqueue.WithPollInterval(10*time.Millisecond))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- b.Run(ctx) }()

	if err := jobs.Enqueue(context.Background(), b, &greetJob{Name: "Ada"}); err != nil {
		fmt.Println("error:", err)
		return
	}

	<-done
	cancel()
	<-runDone

	// Output:
	// hello, Ada
}
