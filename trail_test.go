package trails

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"
)

func newTestTrail(t *testing.T, opts *TrailOptions) *Trail {
	t.Helper()

	if opts.AssetsFS == nil {
		opts.AssetsFS = fstest.MapFS{
			"manifest.json": &fstest.MapFile{Data: []byte("{}")},
		}
	}
	if opts.ConfigFS == nil {
		opts.ConfigFS = fstest.MapFS{
			"importmap.toml": &fstest.MapFile{Data: []byte("")},
		}
	}
	if opts.ViewFS == nil {
		opts.ViewFS = fstest.MapFS{}
	}

	trail, err := New(WithDefaultOptions(opts))
	require.NoError(t, err)

	return trail
}

func TestNewWiresOptionsIntoTrail(t *testing.T) {
	trail := newTestTrail(t, &TrailOptions{Host: "0.0.0.0", Port: 8080})

	require.Equal(t, "0.0.0.0", trail.Host)
	require.Equal(t, 8080, trail.Port)

	c := trail.contextPool.Get().(*Context)
	require.Same(t, trail.logger, c.Logger())
	require.Equal(t, trail.context, c.Context())
}

func TestNewReturnsErrorWhenAssetManifestMissing(t *testing.T) {
	opts := WithDefaultOptions(&TrailOptions{
		AssetsFS: fstest.MapFS{},
		ConfigFS: fstest.MapFS{
			"importmap.toml": &fstest.MapFile{Data: []byte("")},
		},
		ViewFS: fstest.MapFS{},
	})

	trail, err := New(opts)

	require.Nil(t, trail)
	require.Error(t, err)
	require.Contains(t, err.Error(), "loading asset manifest")
}

func TestNewReturnsErrorWhenImportMapConfigMissing(t *testing.T) {
	opts := WithDefaultOptions(&TrailOptions{
		AssetsFS: fstest.MapFS{
			"manifest.json": &fstest.MapFile{Data: []byte("{}")},
		},
		ConfigFS: fstest.MapFS{},
		ViewFS:   fstest.MapFS{},
	})

	trail, err := New(opts)

	require.Nil(t, trail)
	require.Error(t, err)
	require.Contains(t, err.Error(), "loading importmap config")
}

func viewFSWithImportMapsHelper() fstest.MapFS {
	return fstest.MapFS{
		"layouts/application.gohtml": &fstest.MapFile{
			Data: []byte(`{{define "application"}}{{import_maps}}{{template "content" .}}{{end}}`),
		},
		"posts/index.gohtml": &fstest.MapFile{
			Data: []byte(`{{define "content"}}hi{{end}}`),
		},
	}
}

func TestNewSkipsImportMapWhenStrategyIsNone(t *testing.T) {
	opts := WithDefaultOptions(&TrailOptions{
		AssetsFS: fstest.MapFS{
			"manifest.json": &fstest.MapFile{Data: []byte("{}")},
		},
		ConfigFS:       fstest.MapFS{},
		ViewFS:         viewFSWithImportMapsHelper(),
		AssetsStrategy: AssetsStrategyNone,
	})

	trail, err := New(opts)
	require.NoError(t, err)
	require.NotNil(t, trail)

	var buf bytes.Buffer
	require.NoError(t, trail.renderer.Render(nil, &buf, "posts/index", nil))
	require.Equal(t, "hi", buf.String())
}

func TestNewSkipsImportMapWhenStrategyIsBundler(t *testing.T) {
	opts := WithDefaultOptions(&TrailOptions{
		AssetsFS: fstest.MapFS{
			"manifest.json": &fstest.MapFile{Data: []byte("{}")},
		},
		ConfigFS:       fstest.MapFS{},
		ViewFS:         viewFSWithImportMapsHelper(),
		AssetsStrategy: AssetsStrategyBundler,
	})

	trail, err := New(opts)
	require.NoError(t, err)
	require.NotNil(t, trail)

	var buf bytes.Buffer
	require.NoError(t, trail.renderer.Render(nil, &buf, "posts/index", nil))
	require.Equal(t, "hi", buf.String())
}

func TestNewSetsUpRenderer(t *testing.T) {
	trail := newTestTrail(t, &TrailOptions{})

	require.NotNil(t, trail.renderer)
}

func TestNewSetsUpRouter(t *testing.T) {
	trail := newTestTrail(t, &TrailOptions{})

	require.NotNil(t, trail.router)
	require.Same(t, trail.contextPool, trail.router.pool)
	require.Equal(t,
		reflect.ValueOf(trail.errorHandler).Pointer(),
		reflect.ValueOf(trail.router.errorHandler).Pointer(),
	)
}

func TestUseDelegatesToRouter(t *testing.T) {
	trail := newTestTrail(t, &TrailOptions{})
	require.Len(t, trail.router.middleware, 0)

	trail.Use(recordMiddleware("one", &[]string{}))
	require.Len(t, trail.router.middleware, 1)
}

func TestServeHTTPDelegatesToRouter(t *testing.T) {
	trail := newTestTrail(t, &TrailOptions{})
	trail.router.Get("/ok", func(c *Context) error {
		return c.String(http.StatusOK, "hello")
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ok", nil)
	trail.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "hello", w.Body.String())
}

func TestRunReturnsServerErrorWhenAddressInUse(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	addr := listener.Addr().(*net.TCPAddr)
	trail := newTestTrail(t, &TrailOptions{Host: "127.0.0.1", Port: addr.Port})

	err = trail.Run()
	require.Error(t, err)
	require.Contains(t, err.Error(), "server error")
}

func TestRunReturnsShutdownError(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().(*net.TCPAddr)
	require.NoError(t, listener.Close())

	ctx, cancel := context.WithCancel(context.Background())
	trail := newTestTrail(t, &TrailOptions{Host: "127.0.0.1", Port: addr.Port, Context: ctx})

	handlerEntered := make(chan struct{})
	release := make(chan struct{})
	trail.router.Get("/block", func(c *Context) error {
		close(handlerEntered)
		<-release
		return c.String(http.StatusOK, "done")
	})

	result := make(chan error, 1)
	go func() {
		result <- trail.Run()
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		conn, dialErr := net.Dial("tcp", addr.String())
		if dialErr == nil {
			_ = conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server did not start listening in time: %v", dialErr)
		}
		time.Sleep(5 * time.Millisecond)
	}

	target := fmt.Sprintf("http://127.0.0.1:%d/block", addr.Port)
	go func() {
		_, _ = http.Get(target)
	}()

	select {
	case <-handlerEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not receive request in time")
	}

	cancel()

	select {
	case err := <-result:
		require.Error(t, err)
		require.Contains(t, err.Error(), "server shutdown error")
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}

	close(release)
}

func TestRunShutsDownGracefullyOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	trail := newTestTrail(t, &TrailOptions{Host: "127.0.0.1", Port: 0, Context: ctx})

	result := make(chan error, 1)
	go func() {
		result <- trail.Run()
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}
