package cloudtask_test

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	cloudtaskspb "cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
	gax "github.com/googleapis/gax-go/v2"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/idtoken"

	trails "github.com/asimmons91/trails"
	"github.com/asimmons91/trails/jobs/backend/cloudtask"
	"github.com/asimmons91/trails/pack"
	"github.com/asimmons91/trails/pack/driver/sqlite"
	"github.com/asimmons91/trails/pack/migrate"
)

const (
	testServiceAccount = "cloudtask@test.iam.gserviceaccount.com"
	testBaseURL        = "https://example-run-app.a.run.app/cloudtasks"
)

var errCreateTaskRejected = errors.New("cloudtask_test: CreateTask rejected")

func newTestDB(t *testing.T) *pack.DB {
	t.Helper()

	d := sqlite.New()
	sqlDB, err := d.Open(":memory:")
	require.NoError(t, err)
	// Concurrent transactions against ":memory:" each see an independent
	// empty database unless pinned to the one connection that ran
	// migrations — claim.go's tests specifically exercise concurrent
	// transactions.
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	db := pack.Open(sqlDB, d.Dialect())
	require.NoError(t, cloudtask.Migration.Migrate(context.Background(), migrate.New(db)))
	return db
}

func testConfig() cloudtask.Config {
	return cloudtask.Config{
		ProjectID:           "test-project",
		Location:            "us-central1",
		BaseURL:             testBaseURL,
		ServiceAccountEmail: testServiceAccount,
	}
}

// fakeTaskClient records every CreateTask call and, unless err is set,
// returns a synthetic Task so callers can assert on exactly what this
// backend asked Cloud Tasks to do without any network access.
type fakeTaskClient struct {
	mu       sync.Mutex
	requests []*cloudtaskspb.CreateTaskRequest
	err      error                                      // if set, every CreateTask call fails with this error
	failWhen func(*cloudtaskspb.CreateTaskRequest) bool // if set, only calls matching this fail
}

func (f *fakeTaskClient) CreateTask(_ context.Context, req *cloudtaskspb.CreateTaskRequest, _ ...gax.CallOption) (*cloudtaskspb.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.requests = append(f.requests, req)
	if f.err != nil {
		return nil, f.err
	}
	if f.failWhen != nil && f.failWhen(req) {
		return nil, errCreateTaskRejected
	}
	return &cloudtaskspb.Task{Name: req.Parent + "/tasks/fake"}, nil
}

// isCleanupRequest reports whether req is a cleanup-chain push rather than a
// job push, distinguishing the two by target URL.
func isCleanupRequest(req *cloudtaskspb.CreateTaskRequest) bool {
	return req.Task.GetHttpRequest().GetUrl() == testBaseURL+"/tasks/cleanup"
}

// firstRequest is what tests should assert on for a single Enqueue call's
// own job task: Enqueue always creates the job's CreateTask request before
// its best-effort ensureScheduled call, which — the first time it runs —
// itself calls CreateTask again to bootstrap the cleanup chain. Taking the
// most recent request would pick up that second, unrelated call instead.
func (f *fakeTaskClient) firstRequest() *cloudtaskspb.CreateTaskRequest {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.requests) == 0 {
		return nil
	}
	return f.requests[0]
}

func (f *fakeTaskClient) countMatching(pred func(*cloudtaskspb.CreateTaskRequest) bool) int {
	f.mu.Lock()
	defer f.mu.Unlock()

	n := 0
	for _, r := range f.requests {
		if pred(r) {
			n++
		}
	}
	return n
}

// fakeVerifier lets handler tests control the outcome of OIDC verification
// without a real token or network access.
type fakeVerifier struct {
	payload *idtoken.Payload
	err     error
}

func (f fakeVerifier) Validate(context.Context, string, string) (*idtoken.Payload, error) {
	return f.payload, f.err
}

func validVerifier() fakeVerifier {
	return fakeVerifier{payload: payloadWithEmail(testServiceAccount)}
}

func payloadWithEmail(email string) *idtoken.Payload {
	return &idtoken.Payload{Claims: map[string]any{"email": email}}
}

func nowNanos() int64 { return time.Now().UnixNano() }

// mountBackend builds a minimal, fully working *trails.Trail with b's
// routes mounted at the root, so handler tests exercise the same
// request/response path (including trails' Bind machinery) a real Cloud
// Tasks push would go through, without needing real templates or assets.
func mountBackend(t *testing.T, b *cloudtask.Backend) http.Handler {
	t.Helper()

	trail, err := trails.New(trails.WithDefaultOptions(&trails.TrailOptions{
		AssetsFS:       fstest.MapFS{"manifest.json": &fstest.MapFile{Data: []byte("{}")}},
		ConfigFS:       fstest.MapFS{"importmap.toml": &fstest.MapFile{Data: []byte("")}},
		ViewFS:         fstest.MapFS{},
		AssetsStrategy: trails.AssetsStrategyNone,
		RouteBuilder: func(r *trails.Router) {
			r.WithGroup("", b.Routes)
		},
	}))
	require.NoError(t, err)

	return trail
}

// jobRowView, slotRowView, and scheduleRowView give tests a read/write path
// into cloudtask's tables without cloudtask needing to export its own
// internal row types — same approach as dbqueue's driver-submodule tests.
type jobRowView struct {
	pack.Model[int64] `db:"table:cloudtask_jobs"`

	Kind                  string `db:"kind"`
	Args                  string `db:"args"`
	Queue                 string `db:"queue"`
	State                 string `db:"state"`
	Attempts              int    `db:"attempts"`
	MaxAttempts           int    `db:"max_attempts"`
	ConcurrencyKey        string `db:"concurrency_key"`
	ConcurrencyLimit      int    `db:"concurrency_limit"`
	ConcurrencyDurationNs int64  `db:"concurrency_duration_ns"`
	LockedAt              int64  `db:"locked_at"`
	TaskName              string `db:"task_name"`
	LastError             string `db:"last_error"`
	FinishedAt            int64  `db:"finished_at"`
	CreatedAt             int64  `db:"created_at"`
}

type slotRowView struct {
	pack.Model[int64] `db:"table:cloudtask_concurrency_slots"`

	ConcurrencyKey string `db:"concurrency_key"`
	CreatedAt      int64  `db:"created_at"`
}

type scheduleRowView struct {
	pack.Model[int64] `db:"table:cloudtask_schedule"`

	Name      string `db:"name"`
	NextRunAt int64  `db:"next_run_at"`
	CreatedAt int64  `db:"created_at"`
}

// testJob is a jobs.Job whose behavior a test controls via a captured
// closure. perform is unexported, so json.Unmarshal (which Registry.Dispatch
// runs on Args before calling Perform) never touches it.
type testJob struct {
	kind    string
	perform func(context.Context) error
}

func (j *testJob) Kind() string { return j.kind }

func (j *testJob) Perform(ctx context.Context) error {
	if j.perform != nil {
		return j.perform(ctx)
	}
	return nil
}
