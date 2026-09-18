package secureheaders_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	trails "github.com/asimmons91/trails"
	"github.com/asimmons91/trails/secureheaders"
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

func get(t *testing.T, trail *trails.Trail, path string) *httptest.ResponseRecorder {
	t.Helper()

	w := httptest.NewRecorder()
	trail.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func TestMiddlewareDefaultsSetExpectedHeaders(t *testing.T) {
	build := func(r *trails.Router) {
		r.Use(secureheaders.Middleware())
		r.Get("/ok", func(c *trails.Context) error {
			return c.String(http.StatusOK, "ok")
		})
	}
	trail := newTestTrail(t, build)

	w := get(t, trail, "/ok")

	require.Equal(t, "max-age=63072000; includeSubDomains", w.Header().Get("Strict-Transport-Security"))
	require.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	require.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
	require.Equal(t, "strict-origin-when-cross-origin", w.Header().Get("Referrer-Policy"))
	require.Empty(t, w.Header().Get("Content-Security-Policy"))
}

func TestWithHSTSMaxAgeZeroOmitsHeader(t *testing.T) {
	build := func(r *trails.Router) {
		r.Use(secureheaders.Middleware(secureheaders.WithHSTSMaxAge(0)))
		r.Get("/ok", func(c *trails.Context) error { return c.String(http.StatusOK, "ok") })
	}
	trail := newTestTrail(t, build)

	w := get(t, trail, "/ok")

	require.Empty(t, w.Header().Get("Strict-Transport-Security"))
}

func TestWithHSTSPreloadAddsDirective(t *testing.T) {
	build := func(r *trails.Router) {
		r.Use(secureheaders.Middleware(secureheaders.WithHSTSPreload(true)))
		r.Get("/ok", func(c *trails.Context) error { return c.String(http.StatusOK, "ok") })
	}
	trail := newTestTrail(t, build)

	w := get(t, trail, "/ok")

	require.Equal(t, "max-age=63072000; includeSubDomains; preload", w.Header().Get("Strict-Transport-Security"))
}

func TestWithContentTypeNosniffFalseOmitsHeader(t *testing.T) {
	build := func(r *trails.Router) {
		r.Use(secureheaders.Middleware(secureheaders.WithContentTypeNosniff(false)))
		r.Get("/ok", func(c *trails.Context) error { return c.String(http.StatusOK, "ok") })
	}
	trail := newTestTrail(t, build)

	w := get(t, trail, "/ok")

	require.Empty(t, w.Header().Get("X-Content-Type-Options"))
}

func TestWithFrameOptionsOverridesValue(t *testing.T) {
	build := func(r *trails.Router) {
		r.Use(secureheaders.Middleware(secureheaders.WithFrameOptions(secureheaders.FrameOptionsSameOrigin)))
		r.Get("/ok", func(c *trails.Context) error { return c.String(http.StatusOK, "ok") })
	}
	trail := newTestTrail(t, build)

	w := get(t, trail, "/ok")

	require.Equal(t, "SAMEORIGIN", w.Header().Get("X-Frame-Options"))
}

func TestWithFrameOptionsEmptyOmitsHeader(t *testing.T) {
	build := func(r *trails.Router) {
		r.Use(secureheaders.Middleware(secureheaders.WithFrameOptions("")))
		r.Get("/ok", func(c *trails.Context) error { return c.String(http.StatusOK, "ok") })
	}
	trail := newTestTrail(t, build)

	w := get(t, trail, "/ok")

	require.Empty(t, w.Header().Get("X-Frame-Options"))
}

func TestWithReferrerPolicyOverridesValue(t *testing.T) {
	build := func(r *trails.Router) {
		r.Use(secureheaders.Middleware(secureheaders.WithReferrerPolicy("no-referrer")))
		r.Get("/ok", func(c *trails.Context) error { return c.String(http.StatusOK, "ok") })
	}
	trail := newTestTrail(t, build)

	w := get(t, trail, "/ok")

	require.Equal(t, "no-referrer", w.Header().Get("Referrer-Policy"))
}

func TestWithCSPProducesDeterministicSortedOutput(t *testing.T) {
	build := func(r *trails.Router) {
		r.Use(secureheaders.Middleware(secureheaders.WithCSP(map[string][]string{
			"script-src":  {"'self'"},
			"default-src": {"'self'"},
			"img-src":     {"'self'", "data:"},
		})))
		r.Get("/ok", func(c *trails.Context) error { return c.String(http.StatusOK, "ok") })
	}
	trail := newTestTrail(t, build)

	const want = "default-src 'self'; img-src 'self' data:; script-src 'self'"

	for range 5 {
		w := get(t, trail, "/ok")
		require.Equal(t, want, w.Header().Get("Content-Security-Policy"))
	}
}

func TestMiddlewareSetsHeadersOnErrorResponse(t *testing.T) {
	build := func(r *trails.Router) {
		r.Use(secureheaders.Middleware())
		r.Get("/boom", func(c *trails.Context) error {
			return trails.NewHTTPError(http.StatusInternalServerError, errors.New("boom"))
		})
	}
	trail := newTestTrail(t, build)

	w := get(t, trail, "/boom")

	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	require.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
}
