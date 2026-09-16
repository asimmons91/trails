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

// fakeRunnerBroadcaster is a channels.Broadcaster that also implements
// trails.Runner, standing in for database.Backend without pulling in a real
// database driver.
type fakeRunnerBroadcaster struct {
	started chan struct{}
}

func (b *fakeRunnerBroadcaster) Publish(ctx context.Context, topic string, payload []byte) error {
	return nil
}

func (b *fakeRunnerBroadcaster) Subscribe(ctx context.Context, topic string) (channels.Subscription, error) {
	return nil, nil
}

func (b *fakeRunnerBroadcaster) Run(ctx context.Context) error {
	close(b.started)
	<-ctx.Done()
	return nil
}

// fakePlainBroadcaster is a channels.Broadcaster with no Run method, used to
// exercise Backend.Run's fallback for Broadcasters that don't implement
// trails.Runner at all (both memory.Backend and database.Backend do, so
// nothing shipped in this repo exercises this path anymore).
type fakePlainBroadcaster struct{}

func (b *fakePlainBroadcaster) Publish(ctx context.Context, topic string, payload []byte) error {
	return nil
}

func (b *fakePlainBroadcaster) Subscribe(ctx context.Context, topic string) (channels.Subscription, error) {
	return nil, nil
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

func TestRun_DelegatesToRunnableBroadcaster(t *testing.T) {
	bc := &fakeRunnerBroadcaster{started: make(chan struct{})}
	b := channels.New(channels.NewHub(bc, channels.NewRegistry()))

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- b.Run(ctx) }()

	select {
	case <-bc.started:
	case <-time.After(2 * time.Second):
		t.Fatal("broadcaster Run was not started")
	}

	cancel()

	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Backend.Run did not return after context cancellation")
	}
}

func TestRun_NoopsWhenBroadcasterHasNoRunMethod(t *testing.T) {
	b := channels.New(channels.NewHub(&fakePlainBroadcaster{}, channels.NewRegistry()))

	done := make(chan error, 1)
	go func() { done <- b.Run(context.Background()) }()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("Backend.Run should return immediately when the broadcaster has no Run method")
	}
}

func TestRun_DelegatesToMemoryBackendRunUntilContextCancelled(t *testing.T) {
	b := channels.New(newTestHub()) // memory.Backend now implements trails.Runner

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- b.Run(ctx) }()

	// memory.Backend.Run blocks on ctx.Done(); confirm Backend.Run doesn't
	// return early on its own.
	select {
	case err := <-result:
		t.Fatalf("Backend.Run returned before context cancellation: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	cancel()

	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Backend.Run did not return after context cancellation")
	}
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
