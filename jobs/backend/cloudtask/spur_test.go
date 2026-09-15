package cloudtask_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/asimmons91/trails/jobs"
	"github.com/asimmons91/trails/jobs/backend/cloudtask"
)

func TestViewFS_IsNil(t *testing.T) {
	db := newTestDB(t)
	b := cloudtask.New(db, jobs.NewRegistry(), &fakeTaskClient{}, testConfig())

	require.Nil(t, b.ViewFS())
}

func TestAssetsFS_IsNil(t *testing.T) {
	db := newTestDB(t)
	b := cloudtask.New(db, jobs.NewRegistry(), &fakeTaskClient{}, testConfig())

	require.Nil(t, b.AssetsFS())
}

func TestJobs_RegistersNoKinds(t *testing.T) {
	db := newTestDB(t)
	b := cloudtask.New(db, jobs.NewRegistry(), &fakeTaskClient{}, testConfig())

	reg := jobs.NewRegistry()
	b.Jobs(reg)

	require.Empty(t, reg.Schedules())
	err := reg.Dispatch(context.Background(), jobs.Enqueued{Kind: "anything"})
	require.ErrorIs(t, err, jobs.ErrUnknownKind)
}

func TestRoutes_RegistersPushAndCleanupPaths(t *testing.T) {
	db := newTestDB(t)
	b := cloudtask.New(db, jobs.NewRegistry(), &fakeTaskClient{}, testConfig(),
		cloudtask.WithTokenVerifier(validVerifier()))
	mount := mountBackend(t, b)

	for _, path := range []string{"/tasks/run", "/tasks/cleanup"} {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		rec := httptest.NewRecorder()
		mount.ServeHTTP(rec, req)
		require.NotEqual(t, http.StatusNotFound, rec.Code, "expected %s to be routed", path)
	}

	// Only POST should be registered — a GET should not match (Go's
	// method-based ServeMux responds 405, not 404, when the path exists
	// for a different method).
	req := httptest.NewRequest(http.MethodGet, "/tasks/run", nil)
	rec := httptest.NewRecorder()
	mount.ServeHTTP(rec, req)
	require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}
