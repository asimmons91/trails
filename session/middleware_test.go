package session_test

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"testing/fstest"

	trails "github.com/asimmons91/trails"
	"github.com/asimmons91/trails/session"
	"github.com/stretchr/testify/require"
)

// testKey is a valid 32-byte AES-256-GCM key, hex-encoded (64 hex chars) —
// the same shape credentials.GenerateKey() produces.
const testKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
const otherKey = "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"

func newTestTrail(t *testing.T, routeBuilder trails.RouteBuilder, mutate ...func(*trails.TrailOptions)) *trails.Trail {
	t.Helper()

	opts := &trails.TrailOptions{
		ViewFS:       fstest.MapFS{},
		AssetsFS:     fstest.MapFS{"manifest.json": &fstest.MapFile{Data: []byte("{}")}},
		ConfigFS:     fstest.MapFS{"importmap.toml": &fstest.MapFile{Data: []byte("")}},
		RouteBuilder: routeBuilder,
	}
	for _, m := range mutate {
		m(opts)
	}

	trail, err := trails.New(trails.WithDefaultOptions(opts))
	require.NoError(t, err)

	return trail
}

func setCookie(t *testing.T, w *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()

	for _, c := range w.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}

	return nil
}

func TestSessionMiddlewareRoundTripsValues(t *testing.T) {
	build := func(r *trails.Router) {
		r.Use(session.Middleware(testKey))
		r.Post("/set", func(c *trails.Context) error {
			session.FromContext(c).Set("name", "alice")
			return c.String(http.StatusOK, "ok")
		})
		r.Get("/get", func(c *trails.Context) error {
			name, _ := session.FromContext(c).Get("name").(string)
			return c.String(http.StatusOK, "name="+name)
		})
	}
	trail := newTestTrail(t, build)

	w1 := httptest.NewRecorder()
	trail.ServeHTTP(w1, httptest.NewRequest(http.MethodPost, "/set", nil))
	cookie := setCookie(t, w1, "_trails_session")
	require.NotNil(t, cookie)

	r2 := httptest.NewRequest(http.MethodGet, "/get", nil)
	r2.AddCookie(cookie)
	w2 := httptest.NewRecorder()
	trail.ServeHTTP(w2, r2)

	require.Equal(t, "name=alice", w2.Body.String())
}

func TestSessionMiddlewareStartsFreshOnMissingCookie(t *testing.T) {
	build := func(r *trails.Router) {
		r.Use(session.Middleware(testKey))
		r.Get("/get", func(c *trails.Context) error {
			name, _ := session.FromContext(c).Get("name").(string)
			return c.String(http.StatusOK, "name="+name)
		})
	}
	trail := newTestTrail(t, build)

	w := httptest.NewRecorder()
	trail.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/get", nil))

	require.Equal(t, "name=", w.Body.String())
}

func TestSessionMiddlewareIgnoresTamperedCookie(t *testing.T) {
	build := func(r *trails.Router) {
		r.Use(session.Middleware(testKey))
		r.Post("/set", func(c *trails.Context) error {
			session.FromContext(c).Set("name", "alice")
			return c.String(http.StatusOK, "ok")
		})
		r.Get("/get", func(c *trails.Context) error {
			name, _ := session.FromContext(c).Get("name").(string)
			return c.String(http.StatusOK, "name="+name)
		})
	}
	trail := newTestTrail(t, build)

	w1 := httptest.NewRecorder()
	trail.ServeHTTP(w1, httptest.NewRequest(http.MethodPost, "/set", nil))
	cookie := setCookie(t, w1, "_trails_session")
	require.NotNil(t, cookie)
	cookie.Value = cookie.Value[:len(cookie.Value)-4] + "AAAA"

	r2 := httptest.NewRequest(http.MethodGet, "/get", nil)
	r2.AddCookie(cookie)
	w2 := httptest.NewRecorder()
	trail.ServeHTTP(w2, r2)

	require.Equal(t, http.StatusOK, w2.Code)
	require.Equal(t, "name=", w2.Body.String())
}

func TestSessionMiddlewareIgnoresCookieEncryptedWithDifferentKey(t *testing.T) {
	build := func(key string) trails.RouteBuilder {
		return func(r *trails.Router) {
			r.Use(session.Middleware(key))
			r.Post("/set", func(c *trails.Context) error {
				session.FromContext(c).Set("name", "alice")
				return c.String(http.StatusOK, "ok")
			})
			r.Get("/get", func(c *trails.Context) error {
				name, _ := session.FromContext(c).Get("name").(string)
				return c.String(http.StatusOK, "name="+name)
			})
		}
	}

	trailA := newTestTrail(t, build(testKey))
	trailB := newTestTrail(t, build(otherKey))

	w1 := httptest.NewRecorder()
	trailA.ServeHTTP(w1, httptest.NewRequest(http.MethodPost, "/set", nil))
	cookie := setCookie(t, w1, "_trails_session")
	require.NotNil(t, cookie)

	r2 := httptest.NewRequest(http.MethodGet, "/get", nil)
	r2.AddCookie(cookie)
	w2 := httptest.NewRecorder()
	trailB.ServeHTTP(w2, r2)

	require.Equal(t, http.StatusOK, w2.Code)
	require.Equal(t, "name=", w2.Body.String())
}

func TestSessionMiddlewareSkipsSetCookieWhenUnchanged(t *testing.T) {
	build := func(r *trails.Router) {
		r.Use(session.Middleware(testKey))
		r.Get("/noop", func(c *trails.Context) error {
			return c.String(http.StatusOK, "ok")
		})
	}
	trail := newTestTrail(t, build)

	w := httptest.NewRecorder()
	trail.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/noop", nil))

	require.Nil(t, setCookie(t, w, "_trails_session"))
}

func TestSessionMiddlewareSetsCookieBeforeHandlerWritesBody(t *testing.T) {
	build := func(r *trails.Router) {
		r.Use(session.Middleware(testKey))
		r.Get("/set", func(c *trails.Context) error {
			session.FromContext(c).Set("name", "alice")
			// c.String writes the header synchronously — the session
			// commit must happen before this call returns.
			return c.String(http.StatusOK, "ok")
		})
	}
	trail := newTestTrail(t, build)

	w := httptest.NewRecorder()
	trail.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/set", nil))

	require.NotNil(t, setCookie(t, w, "_trails_session"))
}

func TestSessionMiddlewareSetsCookieOnErrorResponsePath(t *testing.T) {
	build := func(r *trails.Router) {
		r.Use(session.Middleware(testKey))
		r.Get("/fail", func(c *trails.Context) error {
			session.FromContext(c).Set("name", "alice")
			return trails.NewHTTPError(http.StatusForbidden, errors.New("nope"))
		})
	}
	trail := newTestTrail(t, build)

	w := httptest.NewRecorder()
	trail.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/fail", nil))

	require.Equal(t, http.StatusForbidden, w.Code)
	require.NotNil(t, setCookie(t, w, "_trails_session"))
}

func TestSessionMiddlewareExpiresCookieOnClear(t *testing.T) {
	build := func(r *trails.Router) {
		r.Use(session.Middleware(testKey))
		r.Post("/set", func(c *trails.Context) error {
			session.FromContext(c).Set("name", "alice")
			return c.String(http.StatusOK, "ok")
		})
		r.Post("/clear", func(c *trails.Context) error {
			session.FromContext(c).Clear()
			return c.String(http.StatusOK, "ok")
		})
	}
	trail := newTestTrail(t, build)

	w1 := httptest.NewRecorder()
	trail.ServeHTTP(w1, httptest.NewRequest(http.MethodPost, "/set", nil))
	cookie := setCookie(t, w1, "_trails_session")
	require.NotNil(t, cookie)

	r2 := httptest.NewRequest(http.MethodPost, "/clear", nil)
	r2.AddCookie(cookie)
	w2 := httptest.NewRecorder()
	trail.ServeHTTP(w2, r2)

	cleared := setCookie(t, w2, "_trails_session")
	require.NotNil(t, cleared)
	require.Less(t, cleared.MaxAge, 0)
	require.Empty(t, cleared.Value)
}

func TestSessionMiddlewareDropsAndLogsOnCookieOverflow(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	build := func(r *trails.Router) {
		r.Use(session.Middleware(testKey, session.WithMaxCookieBytes(32)))
		r.Get("/set", func(c *trails.Context) error {
			session.FromContext(c).Set("name", "a-value-long-enough-to-overflow-a-tiny-cookie-budget")
			return c.String(http.StatusOK, "ok")
		})
	}
	trail := newTestTrail(t, build, func(o *trails.TrailOptions) { o.Logger = logger })

	w := httptest.NewRecorder()
	trail.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/set", nil))

	require.Nil(t, setCookie(t, w, "_trails_session"))
	require.Contains(t, logBuf.String(), "session")
}

func TestSessionMiddlewareDefaultSecureFlagByEnvironment(t *testing.T) {
	cases := []struct {
		name       string
		env        string
		unset      bool
		wantSecure bool
	}{
		{name: "unset falls back to test", unset: true, wantSecure: false},
		{name: "development", env: "development", wantSecure: false},
		{name: "test", env: "test", wantSecure: false},
		{name: "production", env: "production", wantSecure: true},
		{name: "staging", env: "staging", wantSecure: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.unset {
				original, had := os.LookupEnv("TRAILS_ENV")
				require.NoError(t, os.Unsetenv("TRAILS_ENV"))
				t.Cleanup(func() {
					if had {
						os.Setenv("TRAILS_ENV", original)
					}
				})
			} else {
				t.Setenv("TRAILS_ENV", tc.env)
			}

			build := func(r *trails.Router) {
				r.Use(session.Middleware(testKey))
				r.Get("/set", func(c *trails.Context) error {
					session.FromContext(c).Set("name", "alice")
					return c.String(http.StatusOK, "ok")
				})
			}
			trail := newTestTrail(t, build)

			w := httptest.NewRecorder()
			trail.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/set", nil))

			cookie := setCookie(t, w, "_trails_session")
			require.NotNil(t, cookie)
			require.Equal(t, tc.wantSecure, cookie.Secure)
		})
	}
}
