package ratelimit_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	trails "github.com/asimmons91/trails"
	"github.com/asimmons91/trails/cache/backend/memory"
	"github.com/asimmons91/trails/cache/cachetest"
	"github.com/asimmons91/trails/ratelimit"
	"github.com/asimmons91/trails/trustedproxy"
	"github.com/stretchr/testify/require"
)

func newTestTrail(t *testing.T, routeBuilder trails.RouteBuilder) *trails.Trail {
	t.Helper()

	opts := &trails.TrailOptions{
		ViewFS:       fstest.MapFS{},
		AssetsFS:     fstest.MapFS{"manifest.json": &fstest.MapFile{Data: []byte("{}")}},
		ConfigFS:     fstest.MapFS{"importmap.toml": &fstest.MapFile{Data: []byte("")}},
		RouteBuilder: routeBuilder,
	}

	trail, err := trails.New(trails.WithDefaultOptions(opts))
	require.NoError(t, err)

	return trail
}

func get(t *testing.T, trail *trails.Trail, path string, remoteAddr string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, path, nil)
	if remoteAddr != "" {
		req.RemoteAddr = remoteAddr
	}

	w := httptest.NewRecorder()
	trail.ServeHTTP(w, req)
	return w
}

func okHandler(c *trails.Context) error { return c.String(http.StatusOK, "ok") }

func TestAllowsUnderLimitAndBlocksOverIt(t *testing.T) {
	build := func(r *trails.Router) {
		r.Use(ratelimit.Middleware(memory.New(), 2))
		r.Get("/thing", okHandler)
	}
	trail := newTestTrail(t, build)

	require.Equal(t, http.StatusOK, get(t, trail, "/thing", "203.0.113.1:1").Code)
	require.Equal(t, http.StatusOK, get(t, trail, "/thing", "203.0.113.1:1").Code)

	w := get(t, trail, "/thing", "203.0.113.1:1")
	require.Equal(t, http.StatusTooManyRequests, w.Code)
	require.NotEmpty(t, w.Header().Get("Retry-After"))
}

func TestResetsAfterPeriodElapses(t *testing.T) {
	build := func(r *trails.Router) {
		r.Use(ratelimit.Middleware(memory.New(), 1, ratelimit.WithPeriod(20*time.Millisecond)))
		r.Get("/thing", okHandler)
	}
	trail := newTestTrail(t, build)

	require.Equal(t, http.StatusOK, get(t, trail, "/thing", "203.0.113.2:1").Code)
	require.Equal(t, http.StatusTooManyRequests, get(t, trail, "/thing", "203.0.113.2:1").Code)

	time.Sleep(30 * time.Millisecond)

	require.Equal(t, http.StatusOK, get(t, trail, "/thing", "203.0.113.2:1").Code)
}

func TestWithKeyFuncOverrideChangesBucketing(t *testing.T) {
	build := func(r *trails.Router) {
		r.Use(ratelimit.Middleware(memory.New(), 1, ratelimit.WithKeyFunc(func(c *trails.Context) string {
			return "fixed-bucket"
		})))
		r.Get("/thing", okHandler)
	}
	trail := newTestTrail(t, build)

	// Different remote addrs would normally get independent buckets, but
	// the overridden KeyFunc ignores identity entirely, so the second
	// request from a different client still shares the first's bucket.
	require.Equal(t, http.StatusOK, get(t, trail, "/thing", "203.0.113.3:1").Code)
	require.Equal(t, http.StatusTooManyRequests, get(t, trail, "/thing", "203.0.113.4:1").Code)
}

func TestWithNameDisambiguatesStackedMiddlewareButKeepsIdentitySeparate(t *testing.T) {
	build := func(r *trails.Router) {
		r.Use(
			ratelimit.Middleware(memory.New(), 1, ratelimit.WithName("burst")),
			ratelimit.Middleware(memory.New(), 100, ratelimit.WithName("daily")),
		)
		r.Get("/thing", okHandler)
	}
	trail := newTestTrail(t, build)

	// The tight "burst" limiter (1/min) trips before the loose "daily" one
	// (100/min) ever would, proving the two stacked limiters use distinct
	// counters despite sharing a route.
	require.Equal(t, http.StatusOK, get(t, trail, "/thing", "203.0.113.5:1").Code)
	require.Equal(t, http.StatusTooManyRequests, get(t, trail, "/thing", "203.0.113.5:1").Code)

	// A different client under the same WithName-scoped limiter still gets
	// an independent bucket — WithName is additive, not a full override of
	// per-client identity.
	require.Equal(t, http.StatusOK, get(t, trail, "/thing", "203.0.113.6:1").Code)
}

func TestDefaultIdentityUsesTrustedProxyClientIPWhenWired(t *testing.T) {
	build := func(r *trails.Router) {
		r.Use(
			trustedproxy.Middleware(trustedproxy.WithTrustedProxies("10.0.0.0/8")),
			ratelimit.Middleware(memory.New(), 1),
		)
		r.Get("/thing", okHandler)
	}
	trail := newTestTrail(t, build)

	req1 := httptest.NewRequest(http.MethodGet, "/thing", nil)
	req1.RemoteAddr = "10.0.0.1:1"
	req1.Header.Set("X-Forwarded-For", "203.0.113.7")
	w1 := httptest.NewRecorder()
	trail.ServeHTTP(w1, req1)
	require.Equal(t, http.StatusOK, w1.Code)

	// Same trusted-proxy peer, different forwarded client -> independent
	// bucket, proving the limiter keyed on the resolved ClientIP, not the
	// raw peer address (which was identical for both requests).
	req2 := httptest.NewRequest(http.MethodGet, "/thing", nil)
	req2.RemoteAddr = "10.0.0.1:1"
	req2.Header.Set("X-Forwarded-For", "203.0.113.8")
	w2 := httptest.NewRecorder()
	trail.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusOK, w2.Code)

	// The first forwarded client's second request now trips its own bucket.
	req3 := httptest.NewRequest(http.MethodGet, "/thing", nil)
	req3.RemoteAddr = "10.0.0.1:1"
	req3.Header.Set("X-Forwarded-For", "203.0.113.7")
	w3 := httptest.NewRecorder()
	trail.ServeHTTP(w3, req3)
	require.Equal(t, http.StatusTooManyRequests, w3.Code)
}

func TestDefaultIdentityFallsBackToRemoteAddrWithoutTrustedProxy(t *testing.T) {
	build := func(r *trails.Router) {
		r.Use(ratelimit.Middleware(memory.New(), 1))
		r.Get("/thing", okHandler)
	}
	trail := newTestTrail(t, build)

	require.Equal(t, http.StatusOK, get(t, trail, "/thing", "203.0.113.9:1").Code)
	require.Equal(t, http.StatusOK, get(t, trail, "/thing", "203.0.113.10:1").Code)
	require.Equal(t, http.StatusTooManyRequests, get(t, trail, "/thing", "203.0.113.9:2").Code)
}

func TestWithOnLimitedReplacesBuiltinResponse(t *testing.T) {
	build := func(r *trails.Router) {
		r.Use(ratelimit.Middleware(memory.New(), 1, ratelimit.WithOnLimited(
			func(c *trails.Context, retryAfter time.Duration) error {
				return c.String(http.StatusTeapot, "slow down")
			},
		)))
		r.Get("/thing", okHandler)
	}
	trail := newTestTrail(t, build)

	require.Equal(t, http.StatusOK, get(t, trail, "/thing", "203.0.113.11:1").Code)

	w := get(t, trail, "/thing", "203.0.113.11:1")
	require.Equal(t, http.StatusTeapot, w.Code)
	require.Equal(t, "slow down", w.Body.String())
	require.Empty(t, w.Header().Get("Retry-After"), "the custom handler fully replaces the built-in response")
}

func TestIncrementsStoreExactlyOncePerRequest(t *testing.T) {
	store := cachetest.New(memory.New())
	build := func(r *trails.Router) {
		r.Use(ratelimit.Middleware(store, 10))
		r.Get("/thing", okHandler)
	}
	trail := newTestTrail(t, build)

	get(t, trail, "/thing", "203.0.113.12:1")
	get(t, trail, "/thing", "203.0.113.12:1")
	get(t, trail, "/thing", "203.0.113.12:1")

	key := "GET /thing 203.0.113.12"
	require.Equal(t, 3, store.Increments(key))
}
