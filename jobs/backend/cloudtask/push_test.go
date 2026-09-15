package cloudtask_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/asimmons91/trails/jobs"
	"github.com/asimmons91/trails/jobs/backend/cloudtask"
	"github.com/asimmons91/trails/pack"
	"github.com/stretchr/testify/require"
)

func insertJobRow(t *testing.T, db *pack.DB, row *jobRowView) {
	t.Helper()
	require.NoError(t, pack.Create(context.Background(), db, row))
}

func doPush(t *testing.T, handler http.Handler, path string, token string, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestHandlePush_MissingToken_Unauthorized(t *testing.T) {
	db := newTestDB(t)
	b := cloudtask.New(db, jobs.NewRegistry(), &fakeTaskClient{}, testConfig())
	mount := mountBackend(t, b)

	rec := doPush(t, mount, "/tasks/run", "", `{"job_id":1}`)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHandlePush_WrongServiceAccount_Forbidden(t *testing.T) {
	db := newTestDB(t)
	b := cloudtask.New(db, jobs.NewRegistry(), &fakeTaskClient{}, testConfig(),
		cloudtask.WithTokenVerifier(fakeVerifier{payload: payloadWithEmail("someone-else@test.iam.gserviceaccount.com")}))
	mount := mountBackend(t, b)

	rec := doPush(t, mount, "/tasks/run", "tok", `{"job_id":1}`)
	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestHandlePush_MalformedBody_BadRequest(t *testing.T) {
	db := newTestDB(t)
	b := cloudtask.New(db, jobs.NewRegistry(), &fakeTaskClient{}, testConfig(),
		cloudtask.WithTokenVerifier(validVerifier()))
	mount := mountBackend(t, b)

	rec := doPush(t, mount, "/tasks/run", "tok", `not json`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandlePush_UnknownJobID_AcksWithoutDispatch(t *testing.T) {
	db := newTestDB(t)
	b := cloudtask.New(db, jobs.NewRegistry(), &fakeTaskClient{}, testConfig(),
		cloudtask.WithTokenVerifier(validVerifier()))
	mount := mountBackend(t, b)

	rec := doPush(t, mount, "/tasks/run", "tok", `{"job_id":999}`)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestHandlePush_SuccessfulJob_AcksAndMarksFinished(t *testing.T) {
	db := newTestDB(t)
	reg := jobs.NewRegistry()
	ran := false
	reg.Register("k", func() jobs.Job {
		return &testJob{kind: "k", perform: func(context.Context) error { ran = true; return nil }}
	})

	b := cloudtask.New(db, reg, &fakeTaskClient{}, testConfig(), cloudtask.WithTokenVerifier(validVerifier()))
	mount := mountBackend(t, b)

	insertJobRow(t, db, &jobRowView{Kind: "k", Args: "{}", Queue: "default", State: "pending", MaxAttempts: 5})

	rec := doPush(t, mount, "/tasks/run", "tok", `{"job_id":1}`)
	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, ran)

	rows, err := pack.Of[jobRowView](db).Find(context.Background())
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "finished", rows[0].State)
}

func TestHandlePush_FailedJobWithAttemptsRemaining_500AndPending(t *testing.T) {
	db := newTestDB(t)
	reg := jobs.NewRegistry()
	reg.Register("k", func() jobs.Job {
		return &testJob{kind: "k", perform: func(context.Context) error { return errors.New("boom") }}
	})

	b := cloudtask.New(db, reg, &fakeTaskClient{}, testConfig(), cloudtask.WithTokenVerifier(validVerifier()))
	mount := mountBackend(t, b)

	insertJobRow(t, db, &jobRowView{Kind: "k", Args: "{}", Queue: "default", State: "pending", Attempts: 0, MaxAttempts: 3})

	rec := doPush(t, mount, "/tasks/run", "tok", `{"job_id":1}`)
	require.Equal(t, http.StatusInternalServerError, rec.Code)

	rows, err := pack.Of[jobRowView](db).Find(context.Background())
	require.NoError(t, err)
	require.Equal(t, "pending", rows[0].State)
	require.Equal(t, 1, rows[0].Attempts)
	require.NotEmpty(t, rows[0].LastError)
}

func TestHandlePush_FailedJobExhausted_AcksAndMarksFailed(t *testing.T) {
	db := newTestDB(t)
	reg := jobs.NewRegistry()
	reg.Register("k", func() jobs.Job {
		return &testJob{kind: "k", perform: func(context.Context) error { return errors.New("boom") }}
	})

	b := cloudtask.New(db, reg, &fakeTaskClient{}, testConfig(), cloudtask.WithTokenVerifier(validVerifier()))
	mount := mountBackend(t, b)

	insertJobRow(t, db, &jobRowView{Kind: "k", Args: "{}", Queue: "default", State: "pending", Attempts: 2, MaxAttempts: 3})

	rec := doPush(t, mount, "/tasks/run", "tok", `{"job_id":1}`)
	require.Equal(t, http.StatusOK, rec.Code, "queue-level retries must stop once the job's own MaxAttempts is exhausted")

	rows, err := pack.Of[jobRowView](db).Find(context.Background())
	require.NoError(t, err)
	require.Equal(t, "failed", rows[0].State)
	require.Equal(t, 3, rows[0].Attempts)
	require.NotZero(t, rows[0].FinishedAt)
}

func TestHandlePush_ConcurrencyLimitFull_429AndAttemptsUnbumped(t *testing.T) {
	db := newTestDB(t)
	reg := jobs.NewRegistry()
	reg.Register("k", func() jobs.Job { return &testJob{kind: "k"} })

	b := cloudtask.New(db, reg, &fakeTaskClient{}, testConfig(), cloudtask.WithTokenVerifier(validVerifier()))
	mount := mountBackend(t, b)

	// One job of the same concurrency key is already executing, at the limit.
	insertJobRow(t, db, &jobRowView{
		Kind: "k", Args: "{}", Queue: "default", State: "executing",
		ConcurrencyKey: "shared", ConcurrencyLimit: 1, ConcurrencyDurationNs: int64(60_000_000_000),
		LockedAt: nowNanos(),
	})
	insertJobRow(t, db, &jobRowView{
		Kind: "k", Args: "{}", Queue: "default", State: "pending", MaxAttempts: 5,
		ConcurrencyKey: "shared", ConcurrencyLimit: 1, ConcurrencyDurationNs: int64(60_000_000_000),
	})

	rec := doPush(t, mount, "/tasks/run", "tok", `{"job_id":2}`)
	require.Equal(t, http.StatusTooManyRequests, rec.Code)

	rows, err := pack.Of[jobRowView](db).Find(context.Background())
	require.NoError(t, err)
	require.Len(t, rows, 2)
	for _, row := range rows {
		if row.State == "pending" {
			require.Zero(t, row.Attempts, "a deferral due to our own throttling must not consume MaxAttempts")
		}
	}
}
