package channels_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/asimmons91/trails/channels"
	"github.com/asimmons91/trails/channels/backend/memory"
	"github.com/asimmons91/trails/jobs"
)

func newTestHub() *channels.Hub {
	return channels.NewHub(memory.New(), channels.NewRegistry())
}

func TestViewFS_IsNil(t *testing.T) {
	b := channels.New(newTestHub())

	require.Nil(t, b.ViewFS())
}

func TestAssetsFS_IsNil(t *testing.T) {
	b := channels.New(newTestHub())

	require.Nil(t, b.AssetsFS())
}

func TestJobs_RegistersNoKinds(t *testing.T) {
	b := channels.New(newTestHub())

	reg := jobs.NewRegistry()
	b.Jobs(reg)

	require.Empty(t, reg.Schedules())
}

func TestNewPanicsOnNilHub(t *testing.T) {
	require.Panics(t, func() {
		channels.New(nil)
	})
}

func TestRoutes_RegistersStreamAndCommandPaths(t *testing.T) {
	b := channels.New(newTestHub())
	mount := mountBackend(t, "/cable", b)

	// GET /cable is the stream handler: it blocks for the request's
	// lifetime, so give it a short-lived context and confirm it responds
	// with a real SSE stream before the context expires and it returns.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	streamReq := httptest.NewRequest(http.MethodGet, "/cable", nil).WithContext(ctx)
	streamRec := httptest.NewRecorder()
	mount.ServeHTTP(streamRec, streamReq)
	require.Equal(t, http.StatusOK, streamRec.Code)
	require.Equal(t, "text/event-stream", streamRec.Header().Get("Content-Type"))

	// GET is the only method registered at /cable; POST should 405.
	wrongMethod := httptest.NewRequest(http.MethodPost, "/cable", nil)
	wrongMethodRec := httptest.NewRecorder()
	mount.ServeHTTP(wrongMethodRec, wrongMethod)
	require.Equal(t, http.StatusMethodNotAllowed, wrongMethodRec.Code)

	// POST is the only method registered at /cable/command; GET should 405.
	getOnCommand := httptest.NewRequest(http.MethodGet, "/cable/command", nil)
	getOnCommandRec := httptest.NewRecorder()
	mount.ServeHTTP(getOnCommandRec, getOnCommand)
	require.Equal(t, http.StatusMethodNotAllowed, getOnCommandRec.Code)
}
