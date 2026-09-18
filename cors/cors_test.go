package cors_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	trails "github.com/asimmons91/trails"
	"github.com/asimmons91/trails/cors"
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

func buildWithHandlerCounter(mw trails.MiddlewareFunc) (trails.RouteBuilder, *int) {
	calls := 0
	build := func(r *trails.Router) {
		r.Use(mw)
		r.Get("/api/thing", func(c *trails.Context) error {
			calls++
			return c.String(http.StatusOK, "ok")
		})
		r.HandleFunc(http.MethodOptions, "/api/thing", cors.PreflightHandler)
	}
	return build, &calls
}

func TestAllowedOriginGetEchoesOriginAndVaries(t *testing.T) {
	build, _ := buildWithHandlerCounter(cors.Middleware(cors.WithAllowedOrigins("https://app.example.com")))
	trail := newTestTrail(t, build)

	req := httptest.NewRequest(http.MethodGet, "/api/thing", nil)
	req.Header.Set("Origin", "https://app.example.com")

	w := httptest.NewRecorder()
	trail.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "https://app.example.com", w.Header().Get("Access-Control-Allow-Origin"))
	require.Contains(t, w.Header().Values("Vary"), "Origin")
}

func TestDisallowedOriginGetSucceedsWithoutCORSHeaders(t *testing.T) {
	build, calls := buildWithHandlerCounter(cors.Middleware(cors.WithAllowedOrigins("https://app.example.com")))
	trail := newTestTrail(t, build)

	req := httptest.NewRequest(http.MethodGet, "/api/thing", nil)
	req.Header.Set("Origin", "https://evil.example.com")

	w := httptest.NewRecorder()
	trail.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, 1, *calls)
	require.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
}

func TestWildcardOriginWithoutCredentialsReturnsLiteralWildcard(t *testing.T) {
	build, _ := buildWithHandlerCounter(cors.Middleware(cors.WithAllowedOrigins("*")))
	trail := newTestTrail(t, build)

	req := httptest.NewRequest(http.MethodGet, "/api/thing", nil)
	req.Header.Set("Origin", "https://anything.example.com")

	w := httptest.NewRecorder()
	trail.ServeHTTP(w, req)

	require.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
}

func TestWildcardOriginWithCredentialsEchoesRequestOrigin(t *testing.T) {
	build, _ := buildWithHandlerCounter(cors.Middleware(
		cors.WithAllowedOrigins("*"),
		cors.WithAllowCredentials(true),
	))
	trail := newTestTrail(t, build)

	req := httptest.NewRequest(http.MethodGet, "/api/thing", nil)
	req.Header.Set("Origin", "https://anything.example.com")

	w := httptest.NewRecorder()
	trail.ServeHTTP(w, req)

	require.Equal(t, "https://anything.example.com", w.Header().Get("Access-Control-Allow-Origin"))
	require.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
}

func TestPreflightAgainstRegisteredOptionsRouteReturnsNoContentAndSkipsHandler(t *testing.T) {
	build, calls := buildWithHandlerCounter(cors.Middleware(
		cors.WithAllowedOrigins("https://app.example.com"),
		cors.WithMaxAge(600*time.Second),
	))
	trail := newTestTrail(t, build)

	req := httptest.NewRequest(http.MethodOptions, "/api/thing", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)

	w := httptest.NewRecorder()
	trail.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
	require.Equal(t, 0, *calls)
	require.Equal(t, "https://app.example.com", w.Header().Get("Access-Control-Allow-Origin"))
	require.NotEmpty(t, w.Header().Get("Access-Control-Allow-Methods"))
	require.NotEmpty(t, w.Header().Get("Access-Control-Allow-Headers"))
	require.Equal(t, "600", w.Header().Get("Access-Control-Max-Age"))
}

func TestPreflightAgainstUnregisteredOptionsRouteReturnsMethodNotAllowed(t *testing.T) {
	build := func(r *trails.Router) {
		r.Use(cors.Middleware(cors.WithAllowedOrigins("https://app.example.com")))
		r.Get("/api/thing", func(c *trails.Context) error {
			return c.String(http.StatusOK, "ok")
		})
	}
	trail := newTestTrail(t, build)

	req := httptest.NewRequest(http.MethodOptions, "/api/thing", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)

	w := httptest.NewRecorder()
	trail.ServeHTTP(w, req)

	require.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestWithAllowedHeadersWildcardEchoesRequestHeaders(t *testing.T) {
	build, _ := buildWithHandlerCounter(cors.Middleware(
		cors.WithAllowedOrigins("https://app.example.com"),
		cors.WithAllowedHeaders("*"),
	))
	trail := newTestTrail(t, build)

	req := httptest.NewRequest(http.MethodOptions, "/api/thing", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "X-Custom-Header, Content-Type")

	w := httptest.NewRecorder()
	trail.ServeHTTP(w, req)

	require.Equal(t, "X-Custom-Header, Content-Type", w.Header().Get("Access-Control-Allow-Headers"))
}

func TestZeroOptionMiddlewareIsANoOp(t *testing.T) {
	build, calls := buildWithHandlerCounter(cors.Middleware())
	trail := newTestTrail(t, build)

	req := httptest.NewRequest(http.MethodGet, "/api/thing", nil)
	req.Header.Set("Origin", "https://app.example.com")

	w := httptest.NewRecorder()
	trail.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, 1, *calls)
	require.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))

	preflight := httptest.NewRequest(http.MethodOptions, "/api/thing", nil)
	preflight.Header.Set("Origin", "https://app.example.com")
	preflight.Header.Set("Access-Control-Request-Method", http.MethodPost)

	w2 := httptest.NewRecorder()
	trail.ServeHTTP(w2, preflight)

	// The OPTIONS route is bound to cors.PreflightHandler (a no-op), not
	// the counted GET handler, so *calls is unaffected here — this just
	// confirms a disabled Middleware doesn't short-circuit the preflight
	// itself and lets it fall through to the registered handler untouched.
	require.Equal(t, http.StatusOK, w2.Code)
	require.Equal(t, 1, *calls)
	require.Empty(t, w2.Header().Get("Access-Control-Allow-Origin"))
}
