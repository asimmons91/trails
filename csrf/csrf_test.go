package csrf_test

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"

	trails "github.com/asimmons91/trails"
	"github.com/asimmons91/trails/csrf"
	"github.com/asimmons91/trails/session"
	"github.com/stretchr/testify/require"
)

const testKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

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

func withSessionAndCSRF(handlers func(r *trails.Router), csrfOpts ...csrf.Option) trails.RouteBuilder {
	return func(r *trails.Router) {
		r.Use(session.Middleware(testKey), csrf.Middleware(csrfOpts...))
		handlers(r)
	}
}

func sessionCookie(t *testing.T, w *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()

	for _, c := range w.Result().Cookies() {
		if c.Name == "_trails_session" {
			return c
		}
	}

	return nil
}

// csrfField renders csrf.RequestFuncMap's "csrfField" helper for c, exactly
// as the built-in template renderer would when RequestFuncMap is wired into
// TrailOptions.
func csrfField(t *testing.T, c *trails.Context) template.HTML {
	t.Helper()

	fn, ok := csrf.RequestFuncMap(c)["csrfField"].(func() (template.HTML, error))
	require.True(t, ok)

	html, err := fn()
	require.NoError(t, err)

	return html
}

func extractInputValue(t *testing.T, html string) string {
	t.Helper()

	const marker = `value="`
	idx := strings.Index(html, marker)
	require.GreaterOrEqual(t, idx, 0, "no value attribute found in %q", html)

	rest := html[idx+len(marker):]
	end := strings.Index(rest, `"`)
	require.GreaterOrEqual(t, end, 0)

	return rest[:end]
}

func TestCSRFMiddlewareSkipsSafeMethods(t *testing.T) {
	build := withSessionAndCSRF(func(r *trails.Router) {
		r.Get("/read", func(c *trails.Context) error {
			return c.String(http.StatusOK, "ok")
		})
	})
	trail := newTestTrail(t, build)

	w := httptest.NewRecorder()
	trail.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/read", nil))

	require.Equal(t, http.StatusOK, w.Code)
}

func TestCSRFMiddlewareRejectsMissingToken(t *testing.T) {
	build := withSessionAndCSRF(func(r *trails.Router) {
		r.Post("/write", func(c *trails.Context) error {
			return c.String(http.StatusOK, "ok")
		})
	})
	trail := newTestTrail(t, build)

	w := httptest.NewRecorder()
	trail.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/write", nil))

	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestCSRFMiddlewareRejectsMismatchedToken(t *testing.T) {
	build := withSessionAndCSRF(func(r *trails.Router) {
		r.Post("/write", func(c *trails.Context) error {
			return c.String(http.StatusOK, "ok")
		})
	})
	trail := newTestTrail(t, build)

	form := url.Values{"authenticity_token": {"not-a-real-token"}}
	req := httptest.NewRequest(http.MethodPost, "/write", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	trail.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestCSRFMiddlewareRejectsMalformedToken(t *testing.T) {
	build := withSessionAndCSRF(func(r *trails.Router) {
		r.Post("/write", func(c *trails.Context) error {
			return c.String(http.StatusOK, "ok")
		})
	})
	trail := newTestTrail(t, build)

	req := httptest.NewRequest(http.MethodPost, "/write", nil)
	req.Header.Set("X-CSRF-Token", "!!!not-base64!!!")

	w := httptest.NewRecorder()
	trail.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestCSRFMiddlewareAcceptsFormToken(t *testing.T) {
	build := func(r *trails.Router) {
		r.Use(session.Middleware(testKey), csrf.Middleware())
		r.Get("/form", func(c *trails.Context) error {
			return c.HTML(http.StatusOK, string(csrfField(t, c)))
		})
		r.Post("/write", func(c *trails.Context) error {
			return c.String(http.StatusOK, "ok")
		})
	}
	trail := newTestTrail(t, build)

	w1 := httptest.NewRecorder()
	trail.ServeHTTP(w1, httptest.NewRequest(http.MethodGet, "/form", nil))
	cookie := sessionCookie(t, w1)
	require.NotNil(t, cookie)
	token := extractInputValue(t, w1.Body.String())

	form := url.Values{"authenticity_token": {token}}
	req := httptest.NewRequest(http.MethodPost, "/write", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)

	w2 := httptest.NewRecorder()
	trail.ServeHTTP(w2, req)

	require.Equal(t, http.StatusOK, w2.Code)
}

func TestCSRFMiddlewareAcceptsHeaderToken(t *testing.T) {
	build := func(r *trails.Router) {
		r.Use(session.Middleware(testKey), csrf.Middleware())
		r.Get("/form", func(c *trails.Context) error {
			return c.HTML(http.StatusOK, string(csrfField(t, c)))
		})
		r.Post("/write", func(c *trails.Context) error {
			return c.String(http.StatusOK, "ok")
		})
	}
	trail := newTestTrail(t, build)

	w1 := httptest.NewRecorder()
	trail.ServeHTTP(w1, httptest.NewRequest(http.MethodGet, "/form", nil))
	cookie := sessionCookie(t, w1)
	require.NotNil(t, cookie)
	token := extractInputValue(t, w1.Body.String())

	req := httptest.NewRequest(http.MethodPost, "/write", nil)
	req.Header.Set("X-CSRF-Token", token)
	req.AddCookie(cookie)

	w2 := httptest.NewRecorder()
	trail.ServeHTTP(w2, req)

	require.Equal(t, http.StatusOK, w2.Code)
}

func TestCSRFMiddlewarePanicsWithoutSessionMiddleware(t *testing.T) {
	build := func(r *trails.Router) {
		r.Use(csrf.Middleware())
		r.Get("/read", func(c *trails.Context) error {
			return c.String(http.StatusOK, "ok")
		})
	}
	trail := newTestTrail(t, build)

	require.Panics(t, func() {
		w := httptest.NewRecorder()
		trail.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/read", nil))
	})
}

func TestCSRFRequestFuncMapProducesRoundTrippableToken(t *testing.T) {
	build := func(r *trails.Router) {
		r.Use(session.Middleware(testKey), csrf.Middleware())
		r.Get("/form", func(c *trails.Context) error {
			return c.HTML(http.StatusOK, string(csrfField(t, c)))
		})
		r.Post("/write", func(c *trails.Context) error {
			return c.String(http.StatusOK, "ok")
		})
	}
	trail := newTestTrail(t, build)

	w1 := httptest.NewRecorder()
	trail.ServeHTTP(w1, httptest.NewRequest(http.MethodGet, "/form", nil))
	cookie := sessionCookie(t, w1)
	require.NotNil(t, cookie)
	token := extractInputValue(t, w1.Body.String())
	require.NotEmpty(t, token)

	form := url.Values{"authenticity_token": {token}}
	req := httptest.NewRequest(http.MethodPost, "/write", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)

	w2 := httptest.NewRecorder()
	trail.ServeHTTP(w2, req)

	require.Equal(t, http.StatusOK, w2.Code)
}

func TestCSRFRequestFuncMapMasksDifferentlyPerCall(t *testing.T) {
	build := func(r *trails.Router) {
		r.Use(session.Middleware(testKey), csrf.Middleware())
		r.Get("/form", func(c *trails.Context) error {
			first := extractInputValue(t, string(csrfField(t, c)))
			second := extractInputValue(t, string(csrfField(t, c)))
			return c.String(http.StatusOK, first+"|"+second)
		})
	}
	trail := newTestTrail(t, build)

	w := httptest.NewRecorder()
	trail.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/form", nil))

	parts := strings.SplitN(w.Body.String(), "|", 2)
	require.Len(t, parts, 2)
	require.NotEqual(t, parts[0], parts[1])
}

func TestSessionThenCSRFMiddlewareOrderingEndToEnd(t *testing.T) {
	build := func(r *trails.Router) {
		r.Use(session.Middleware(testKey), csrf.Middleware())
		r.Get("/form", func(c *trails.Context) error {
			return c.HTML(http.StatusOK, string(csrfField(t, c)))
		})
		r.Post("/write", func(c *trails.Context) error {
			session.FromContext(c).Set("wrote", true)
			return c.String(http.StatusOK, "ok")
		})
	}
	trail := newTestTrail(t, build)

	w1 := httptest.NewRecorder()
	trail.ServeHTTP(w1, httptest.NewRequest(http.MethodGet, "/form", nil))
	cookie := sessionCookie(t, w1)
	token := extractInputValue(t, w1.Body.String())

	form := url.Values{"authenticity_token": {token}}
	req := httptest.NewRequest(http.MethodPost, "/write", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)

	w2 := httptest.NewRecorder()
	trail.ServeHTTP(w2, req)

	require.Equal(t, http.StatusOK, w2.Code)
}
