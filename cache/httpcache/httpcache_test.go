package httpcache_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	trails "github.com/asimmons91/trails"
	"github.com/asimmons91/trails/cache/backend/memory"
	"github.com/asimmons91/trails/cache/httpcache"
	"github.com/stretchr/testify/require"
)

func newTestTrail(t *testing.T, mw trails.MiddlewareFunc, calls *int, status int) *trails.Trail {
	t.Helper()

	trail, err := trails.New(trails.WithDefaultOptions(&trails.TrailOptions{
		ViewFS: fstest.MapFS{},
		AssetsFS: fstest.MapFS{
			"manifest.json": &fstest.MapFile{Data: []byte("{}")},
		},
		ConfigFS: fstest.MapFS{
			"importmap.toml": &fstest.MapFile{Data: []byte("")},
		},
		RouteBuilder: func(r *trails.Router) {
			r.Use(mw)
			r.Get("/greet", func(c *trails.Context) error {
				*calls++
				return c.String(status, "hit")
			})
			r.Post("/greet", func(c *trails.Context) error {
				*calls++
				return c.String(status, "hit")
			})
		},
	}))
	require.NoError(t, err)

	return trail
}

func TestMiddlewareCachesRepeatedGET(t *testing.T) {
	store := memory.New()
	calls := 0
	trail := newTestTrail(t, httpcache.Middleware(store), &calls, http.StatusOK)

	w1 := httptest.NewRecorder()
	trail.ServeHTTP(w1, httptest.NewRequest(http.MethodGet, "/greet", nil))
	require.Equal(t, "hit", w1.Body.String())
	require.Equal(t, 1, calls)

	w2 := httptest.NewRecorder()
	trail.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/greet", nil))
	require.Equal(t, "hit", w2.Body.String())
	require.Equal(t, 1, calls, "handler must not run again on a cache hit")
	require.NotEmpty(t, w2.Header().Get("ETag"))
}

func TestMiddlewareServes304ForMatchingIfNoneMatch(t *testing.T) {
	store := memory.New()
	calls := 0
	trail := newTestTrail(t, httpcache.Middleware(store), &calls, http.StatusOK)

	w1 := httptest.NewRecorder()
	trail.ServeHTTP(w1, httptest.NewRequest(http.MethodGet, "/greet", nil))
	etag := w1.Header().Get("ETag")
	require.NotEmpty(t, etag)

	req := httptest.NewRequest(http.MethodGet, "/greet", nil)
	req.Header.Set("If-None-Match", etag)
	w2 := httptest.NewRecorder()
	trail.ServeHTTP(w2, req)

	require.Equal(t, http.StatusNotModified, w2.Code)
	require.Empty(t, w2.Body.String())
	require.Equal(t, 1, calls)
}

func TestMiddlewareDoesNotCacheNonGET(t *testing.T) {
	store := memory.New()
	calls := 0
	trail := newTestTrail(t, httpcache.Middleware(store), &calls, http.StatusOK)

	trail.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/greet", nil))
	trail.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/greet", nil))

	require.Equal(t, 2, calls)
}

func TestMiddlewareDoesNotCacheNonOKStatus(t *testing.T) {
	store := memory.New()
	calls := 0
	trail := newTestTrail(t, httpcache.Middleware(store), &calls, http.StatusNotFound)

	trail.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/greet", nil))
	w2 := httptest.NewRecorder()
	trail.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/greet", nil))

	require.Equal(t, http.StatusNotFound, w2.Code)
	require.Equal(t, 2, calls)
}

func TestMiddlewareVariesKeyByQuery(t *testing.T) {
	store := memory.New()
	calls := 0
	trail := newTestTrail(t, httpcache.Middleware(store), &calls, http.StatusOK)

	trail.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/greet?id=1", nil))
	trail.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/greet?id=2", nil))

	require.Equal(t, 2, calls)
}

func TestMiddlewareRespectsCustomTTL(t *testing.T) {
	store := memory.New()
	calls := 0
	trail := newTestTrail(t, httpcache.Middleware(store, httpcache.WithTTL(time.Nanosecond)), &calls, http.StatusOK)

	trail.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/greet", nil))
	time.Sleep(time.Millisecond)
	trail.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/greet", nil))

	require.Equal(t, 2, calls, "expired entry must be treated as a miss")
}

func TestMiddlewareCustomKeyFuncSharesCacheAcrossQueries(t *testing.T) {
	store := memory.New()
	calls := 0
	mw := httpcache.Middleware(store, httpcache.WithKeyFunc(func(r *http.Request) string {
		return r.URL.Path
	}))
	trail := newTestTrail(t, mw, &calls, http.StatusOK)

	trail.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/greet?id=1", nil))
	trail.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/greet?id=2", nil))

	require.Equal(t, 1, calls, "identical path-only key must share the cache entry regardless of query")
}
