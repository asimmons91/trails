package cloudtask_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing/fstest"

	trails "github.com/asimmons91/trails"
	"github.com/asimmons91/trails/jobs"
	"github.com/asimmons91/trails/jobs/backend/cloudtask"
	"github.com/asimmons91/trails/pack"
	"github.com/asimmons91/trails/pack/driver/sqlite"
	"github.com/asimmons91/trails/pack/migrate"
)

// mountedTrail is example_test.go's own t-free equivalent of
// helpers_test.go's mountBackend, needed because a runnable Example takes
// no *testing.T to pass to require.NoError.
func mountedTrail(b *cloudtask.Backend) (http.Handler, error) {
	return trails.New(trails.WithDefaultOptions(&trails.TrailOptions{
		AssetsFS:       fstest.MapFS{"manifest.json": &fstest.MapFile{Data: []byte("{}")}},
		ConfigFS:       fstest.MapFS{"importmap.toml": &fstest.MapFile{Data: []byte("")}},
		ViewFS:         fstest.MapFS{},
		AssetsStrategy: trails.AssetsStrategyNone,
		RouteBuilder: func(r *trails.Router) {
			r.WithGroup("", b.Routes)
		},
	}))
}

// Example simulates a full round trip without any real GCP access: a job
// is enqueued (which would normally create a real Cloud Task), then the
// push Cloud Tasks would later deliver is simulated directly against the
// mounted handler, exactly as push_test.go's handler tests do.
func Example() {
	sqlDB, err := sqlite.New().Open(":memory:")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	defer sqlDB.Close()

	db := pack.Open(sqlDB, sqlite.New().Dialect())
	if err := cloudtask.Migration.Migrate(context.Background(), migrate.New(db)); err != nil {
		fmt.Println("error:", err)
		return
	}

	ran := false
	reg := jobs.NewRegistry()
	reg.Register("greet", func() jobs.Job {
		return &testJob{kind: "greet", perform: func(context.Context) error { ran = true; return nil }}
	})

	b := cloudtask.New(db, reg, &fakeTaskClient{}, testConfig(), cloudtask.WithTokenVerifier(validVerifier()))

	trail, err := mountedTrail(b)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	if err := jobs.Enqueue(context.Background(), b, &greetJob{Name: "Ada"}); err != nil {
		fmt.Println("error:", err)
		return
	}

	// Simulate the Cloud Tasks push Enqueue's CreateTask call would later
	// trigger: a POST to the job-execution handler with a valid bearer
	// token and the row's ID.
	req := httptest.NewRequest(http.MethodPost, "/tasks/run", strings.NewReader(`{"job_id":1}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tok")

	rec := httptest.NewRecorder()
	trail.ServeHTTP(rec, req)

	fmt.Println("status:", rec.Code)
	fmt.Println("ran:", ran)

	// Output:
	// status: 200
	// ran: true
}

type greetJob struct {
	Name string
}

func (j *greetJob) Kind() string { return "greet" }

func (j *greetJob) Perform(context.Context) error { return nil }
