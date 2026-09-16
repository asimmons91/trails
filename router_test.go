package trails

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

func newTestRouter() *Router {
	pool := &sync.Pool{}
	pool.New = func() any { return newContext(nil, nil, nil) }

	return &Router{
		mux:          http.NewServeMux(),
		errorHandler: defaultErrorHandler,
		middleware:   []MiddlewareFunc{},
		pool:         pool,
	}
}

type stubResource struct {
	calls *[]string
}

func (s *stubResource) New(c *Context) error {
	*s.calls = append(*s.calls, "New")
	return c.String(http.StatusOK, "New")
}

func (s *stubResource) Create(c *Context) error {
	*s.calls = append(*s.calls, "Create")
	return c.String(http.StatusOK, "Create")
}

func (s *stubResource) Show(c *Context) error {
	*s.calls = append(*s.calls, "Show")
	return c.String(http.StatusOK, "Show")
}

func (s *stubResource) Edit(c *Context) error {
	*s.calls = append(*s.calls, "Edit")
	return c.String(http.StatusOK, "Edit")
}

func (s *stubResource) Update(c *Context) error {
	*s.calls = append(*s.calls, "Update")
	return c.String(http.StatusOK, "Update")
}

func (s *stubResource) Destroy(c *Context) error {
	*s.calls = append(*s.calls, "Destroy")
	return c.String(http.StatusOK, "Destroy:"+c.Request().PathValue("id"))
}

type stubResources struct {
	stubResource
}

func (s *stubResources) Index(c *Context) error {
	*s.calls = append(*s.calls, "Index")
	return c.String(http.StatusOK, "Index")
}

func recordMiddleware(name string, order *[]string) MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			*order = append(*order, name)
			return next(c)
		}
	}
}

func TestChainOrder(t *testing.T) {
	var order []string

	base := HandlerFunc(func(c *Context) error {
		order = append(order, "base")
		return nil
	})

	mws := []MiddlewareFunc{
		recordMiddleware("A", &order),
		recordMiddleware("B", &order),
		recordMiddleware("C", &order),
	}

	h := chain(base, mws)
	require.NoError(t, h(nil))
	require.Equal(t, []string{"A", "B", "C", "base"}, order)
}

func TestChainEmptyMiddleware(t *testing.T) {
	called := false
	base := HandlerFunc(func(c *Context) error {
		called = true
		return nil
	})

	h := chain(base, nil)
	require.NoError(t, h(nil))
	require.True(t, called)
}

func TestMethodOverrideNoOverride(t *testing.T) {
	rt := newTestRouter()
	r := httptest.NewRequest(http.MethodPost, "/", nil)

	out := rt.methodOverride(r)
	require.Equal(t, http.MethodPost, out.Method)
}

func TestMethodOverrideFormValue(t *testing.T) {
	rt := newTestRouter()
	form := url.Values{"_method": {http.MethodPut}}
	r := httptest.NewRequest(http.MethodPost, "/?"+form.Encode(), nil)

	out := rt.methodOverride(r)
	require.Equal(t, http.MethodPut, out.Method)
}

func TestMethodOverrideHeader(t *testing.T) {
	rt := newTestRouter()
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set("X-Http-Method-Override", http.MethodPatch)

	out := rt.methodOverride(r)
	require.Equal(t, http.MethodPatch, out.Method)
}

func TestMethodOverrideFormTakesPrecedenceOverHeader(t *testing.T) {
	rt := newTestRouter()
	form := url.Values{"_method": {http.MethodDelete}}
	r := httptest.NewRequest(http.MethodPost, "/?"+form.Encode(), nil)
	r.Header.Set("X-Http-Method-Override", http.MethodPatch)

	out := rt.methodOverride(r)
	require.Equal(t, http.MethodDelete, out.Method)
}

func TestMethodOverrideUnrecognizedValue(t *testing.T) {
	rt := newTestRouter()
	form := url.Values{"_method": {"FOO"}}
	r := httptest.NewRequest(http.MethodPost, "/?"+form.Encode(), nil)

	out := rt.methodOverride(r)
	require.Equal(t, http.MethodPost, out.Method)
}

func TestMethodOverrideAllRecognizedMethods(t *testing.T) {
	methods := []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
	}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			rt := newTestRouter()
			form := url.Values{"_method": {method}}
			r := httptest.NewRequest(http.MethodPost, "/?"+form.Encode(), nil)

			out := rt.methodOverride(r)
			require.Equal(t, method, out.Method)
		})
	}
}

func TestShimSuccess(t *testing.T) {
	rt := newTestRouter()
	rt.Get("/ok", func(c *Context) error {
		return c.String(http.StatusOK, "hello")
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/ok", nil)
	rt.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "hello", w.Body.String())
}

func TestShimGenericError(t *testing.T) {
	rt := newTestRouter()
	rt.Get("/fail", func(c *Context) error {
		return errors.New("boom")
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/fail", nil)
	rt.ServeHTTP(w, r)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.True(t, strings.Contains(w.Body.String(), "boom"))
}

func TestShimHTTPError(t *testing.T) {
	rt := newTestRouter()
	rt.Get("/teapot", func(c *Context) error {
		return NewHTTPError(http.StatusTeapot, errors.New("no coffee"))
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/teapot", nil)
	rt.ServeHTTP(w, r)

	require.Equal(t, http.StatusTeapot, w.Code)
	require.True(t, strings.Contains(w.Body.String(), "no coffee"))
}

func TestShimCustomErrorHandler(t *testing.T) {
	rt := newTestRouter()

	called := false
	rt.errorHandler = func(c *Context, err HTTPError) {
		called = true
		_ = c.String(http.StatusBadGateway, "custom: "+err.Error())
	}

	rt.Get("/fail", func(c *Context) error {
		return errors.New("boom")
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/fail", nil)
	rt.ServeHTTP(w, r)

	require.True(t, called)
	require.Equal(t, http.StatusBadGateway, w.Code)
	require.Equal(t, "custom: boom", w.Body.String())
}

func TestShimContextResetBetweenRequests(t *testing.T) {
	rt := newTestRouter()

	rt.Get("/set", func(c *Context) error {
		c.Set("key", "value")
		return c.String(http.StatusOK, "set")
	})

	rt.Get("/read", func(c *Context) error {
		v := c.Get[string]("key")
		return c.String(http.StatusOK, "value="+v)
	})

	w1 := httptest.NewRecorder()
	r1 := httptest.NewRequest(http.MethodGet, "/set", nil)
	rt.ServeHTTP(w1, r1)
	require.Equal(t, "set", w1.Body.String())

	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodGet, "/read", nil)
	rt.ServeHTTP(w2, r2)
	require.Equal(t, "value=", w2.Body.String())
}

func TestHandleFuncConvenienceMethods(t *testing.T) {
	cases := []struct {
		name     string
		register func(rt *Router, pattern string, h HandlerFunc)
		method   string
	}{
		{"Get", (*Router).Get, http.MethodGet},
		{"Post", (*Router).Post, http.MethodPost},
		{"Put", (*Router).Put, http.MethodPut},
		{"Patch", (*Router).Patch, http.MethodPatch},
		{"Delete", (*Router).Delete, http.MethodDelete},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := newTestRouter()
			tc.register(rt, "/resource", func(c *Context) error {
				return c.String(http.StatusOK, tc.method)
			})

			w := httptest.NewRecorder()
			r := httptest.NewRequest(tc.method, "/resource", nil)
			rt.ServeHTTP(w, r)

			require.Equal(t, http.StatusOK, w.Code)
			require.Equal(t, tc.method, w.Body.String())
		})
	}
}

func TestHandleFuncAppliesRouterMiddleware(t *testing.T) {
	rt := newTestRouter()
	var order []string

	rt.Use(recordMiddleware("mw1", &order))
	rt.Use(recordMiddleware("mw2", &order))

	rt.Get("/mw", func(c *Context) error {
		order = append(order, "handler")
		return c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/mw", nil)
	rt.ServeHTTP(w, r)

	require.Equal(t, []string{"mw1", "mw2", "handler"}, order)
}

func TestServeHTTPUsesMethodOverride(t *testing.T) {
	rt := newTestRouter()
	rt.Put("/override", func(c *Context) error {
		return c.String(http.StatusOK, "put-handler")
	})

	form := url.Values{"_method": {http.MethodPut}}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/override?"+form.Encode(), nil)
	rt.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "put-handler", w.Body.String())
}

func TestServeHTTPWithoutOverrideDispatchesNormally(t *testing.T) {
	rt := newTestRouter()
	rt.Get("/normal", func(c *Context) error {
		return c.String(http.StatusOK, "get-handler")
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/normal", nil)
	rt.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "get-handler", w.Body.String())
}

func TestRouterStaticServesFiles(t *testing.T) {
	fsys := fstest.MapFS{
		"app.css": &fstest.MapFile{Data: []byte("body {}")},
	}

	rt := newTestRouter()
	rt.Static("/assets/", fsys)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/assets/app.css", nil)
	rt.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "body {}", w.Body.String())
}

func TestRouterStaticMissingFileReturnsNotFound(t *testing.T) {
	fsys := fstest.MapFS{}

	rt := newTestRouter()
	rt.Static("/assets/", fsys)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/assets/missing.css", nil)
	rt.ServeHTTP(w, r)

	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestNewGroup(t *testing.T) {
	rt := newTestRouter()
	mw := recordMiddleware("g", &[]string{})

	g := rt.NewGroup("/api", mw)

	require.Equal(t, "/api", g.prefix)
	require.Same(t, rt, g.router)
	require.Len(t, g.middleware, 1)
}

func TestNewGroupRoutesReachableUnderPrefix(t *testing.T) {
	rt := newTestRouter()
	g := rt.NewGroup("/api")
	g.Get("/users", func(c *Context) error {
		return c.String(http.StatusOK, "users")
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	rt.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "users", w.Body.String())
}

func TestWithGroup(t *testing.T) {
	rt := newTestRouter()
	var capturedPrefix string

	rt.WithGroup("/admin", func(g *Group) {
		capturedPrefix = g.prefix
		g.Get("/dashboard", func(c *Context) error {
			return c.String(http.StatusOK, "dashboard")
		})
	})

	require.Equal(t, "/admin", capturedPrefix)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/admin/dashboard", nil)
	rt.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "dashboard", w.Body.String())
}

func TestUseAppendsMiddleware(t *testing.T) {
	rt := newTestRouter()
	require.Len(t, rt.middleware, 0)

	rt.Use(recordMiddleware("one", &[]string{}))
	require.Len(t, rt.middleware, 1)

	rt.Use(recordMiddleware("two", &[]string{}), recordMiddleware("three", &[]string{}))
	require.Len(t, rt.middleware, 3)
}

func TestRouterResourceRegistersRoutes(t *testing.T) {
	cases := []struct {
		name     string
		method   string
		path     string
		wantBody string
	}{
		{"New", http.MethodGet, "/profile/new", "New"},
		{"Create", http.MethodPost, "/profile", "Create"},
		{"Show", http.MethodGet, "/profile", "Show"},
		{"Edit", http.MethodPut, "/profile/edit", "Edit"},
		{"Destroy", http.MethodDelete, "/profile", "Destroy:"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := newTestRouter()
			var calls []string
			rt.Resource("/profile", &stubResource{&calls})

			w := httptest.NewRecorder()
			r := httptest.NewRequest(tc.method, tc.path, nil)
			rt.ServeHTTP(w, r)

			require.Equal(t, http.StatusOK, w.Code)
			require.Equal(t, tc.wantBody, w.Body.String())
			require.Equal(t, []string{tc.name}, calls)
		})
	}
}

func TestRouterResourceAppliesMiddleware(t *testing.T) {
	rt := newTestRouter()
	var order []string
	var calls []string

	rt.Resource("/profile", &stubResource{&calls}, recordMiddleware("mw", &order))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/profile", nil)
	rt.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, []string{"mw"}, order)
	require.Equal(t, []string{"Create"}, calls)
}

func TestRouterResourcesRegistersRoutes(t *testing.T) {
	cases := []struct {
		name     string
		method   string
		path     string
		wantBody string
	}{
		{"Index", http.MethodGet, "/users", "Index"},
		{"New", http.MethodGet, "/users/new", "New"},
		{"Create", http.MethodPost, "/users", "Create"},
		{"Show", http.MethodGet, "/users/42", "Show"},
		{"Edit", http.MethodGet, "/users/42/edit", "Edit"},
		{"Update", http.MethodPut, "/users/42", "Update"},
		{"Destroy", http.MethodDelete, "/users/42", "Destroy:42"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := newTestRouter()
			var calls []string
			rt.Resources("/users", &stubResources{stubResource{&calls}})

			w := httptest.NewRecorder()
			r := httptest.NewRequest(tc.method, tc.path, nil)
			rt.ServeHTTP(w, r)

			require.Equal(t, http.StatusOK, w.Code)
			require.Equal(t, tc.wantBody, w.Body.String())
			require.Equal(t, []string{tc.name}, calls)
		})
	}
}

func TestRouterResourcesAppliesMiddleware(t *testing.T) {
	rt := newTestRouter()
	var order []string
	var calls []string

	rt.Resources("/users", &stubResources{stubResource{&calls}}, recordMiddleware("mw", &order))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/users", nil)
	rt.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, []string{"mw"}, order)
	require.Equal(t, []string{"Index"}, calls)
}
