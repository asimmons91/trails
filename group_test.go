package trails

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupHandleFuncRegistersWithPrefix(t *testing.T) {
	rt := newTestRouter()
	g := rt.NewGroup("/api")
	g.HandleFunc(http.MethodGet, "/users", func(c *Context) error {
		return c.String(http.StatusOK, "users")
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	rt.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "users", w.Body.String())
}

func TestGroupHandleFuncAppliesGroupMiddleware(t *testing.T) {
	rt := newTestRouter()
	var order []string

	g := rt.NewGroup("/api", recordMiddleware("A", &order), recordMiddleware("B", &order))
	g.HandleFunc(http.MethodGet, "/thing", func(c *Context) error {
		order = append(order, "handler")
		return c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/thing", nil)
	rt.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, []string{"A", "B", "handler"}, order)
}

func TestGroupNewGroupCombinesPrefixAndMiddleware(t *testing.T) {
	rt := newTestRouter()
	parent := rt.NewGroup("/api", recordMiddleware("A", &[]string{}))

	child := parent.NewGroup("/v1", recordMiddleware("B", &[]string{}))

	require.Equal(t, "/api/v1", child.prefix)
	require.Same(t, rt, child.router)
	require.Len(t, child.middleware, 2)
}

func TestGroupNewGroupRoutesReachableUnderCombinedPrefix(t *testing.T) {
	rt := newTestRouter()
	parent := rt.NewGroup("/api")
	child := parent.NewGroup("/v1")

	child.Get("/resource", func(c *Context) error {
		return c.String(http.StatusOK, "resource")
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/resource", nil)
	rt.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "resource", w.Body.String())
}

func TestGroupNewGroupDoesNotMutateParent(t *testing.T) {
	rt := newTestRouter()
	parent := rt.NewGroup("/api", recordMiddleware("A", &[]string{}))

	child := parent.NewGroup("/v1", recordMiddleware("B", &[]string{}))
	require.Len(t, parent.middleware, 1)
	require.Len(t, child.middleware, 2)

	parent.Use(recordMiddleware("C", &[]string{}))
	require.Len(t, parent.middleware, 2)
	require.Len(t, child.middleware, 2)

	child.Use(recordMiddleware("D", &[]string{}))
	require.Len(t, parent.middleware, 2)
	require.Len(t, child.middleware, 3)
}

func TestGroupWithGroupInvokesBuilderWithCombinedPrefixAndMiddleware(t *testing.T) {
	rt := newTestRouter()
	parent := rt.NewGroup("/api", recordMiddleware("A", &[]string{}))

	var captured *Group
	parent.WithGroup("/nested", func(g *Group) {
		captured = g
		g.Get("/dashboard", func(c *Context) error {
			return c.String(http.StatusOK, "dashboard")
		})
	}, recordMiddleware("B", &[]string{}))

	require.NotNil(t, captured)
	require.Equal(t, "/api/nested", captured.prefix)
	require.Len(t, captured.middleware, 2)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/nested/dashboard", nil)
	rt.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "dashboard", w.Body.String())
}

func TestGroupUseAppendsMiddleware(t *testing.T) {
	rt := newTestRouter()
	g := rt.NewGroup("/api")
	require.Len(t, g.middleware, 0)

	g.Use(recordMiddleware("one", &[]string{}))
	require.Len(t, g.middleware, 1)

	g.Use(recordMiddleware("two", &[]string{}), recordMiddleware("three", &[]string{}))
	require.Len(t, g.middleware, 3)
}

func TestGroupUseAfterHandleFuncDoesNotAffectExistingRoutes(t *testing.T) {
	rt := newTestRouter()
	g := rt.NewGroup("/api")

	var order []string
	g.Get("/first", func(c *Context) error {
		order = append(order, "first-handler")
		return c.String(http.StatusOK, "first")
	})

	g.Use(recordMiddleware("late", &order))

	g.Get("/second", func(c *Context) error {
		order = append(order, "second-handler")
		return c.String(http.StatusOK, "second")
	})

	w1 := httptest.NewRecorder()
	r1 := httptest.NewRequest(http.MethodGet, "/api/first", nil)
	rt.ServeHTTP(w1, r1)
	require.Equal(t, []string{"first-handler"}, order)

	order = nil
	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodGet, "/api/second", nil)
	rt.ServeHTTP(w2, r2)
	require.Equal(t, []string{"late", "second-handler"}, order)
}

func TestGroupConvenienceMethods(t *testing.T) {
	cases := []struct {
		name     string
		register func(g *Group, pattern string, h HandlerFunc)
		method   string
	}{
		{"Get", (*Group).Get, http.MethodGet},
		{"Post", (*Group).Post, http.MethodPost},
		{"Put", (*Group).Put, http.MethodPut},
		{"Patch", (*Group).Patch, http.MethodPatch},
		{"Delete", (*Group).Delete, http.MethodDelete},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := newTestRouter()
			g := rt.NewGroup("/api")
			tc.register(g, "/resource", func(c *Context) error {
				return c.String(http.StatusOK, tc.method)
			})

			w := httptest.NewRecorder()
			r := httptest.NewRequest(tc.method, "/api/resource", nil)
			rt.ServeHTTP(w, r)

			require.Equal(t, http.StatusOK, w.Code)
			require.Equal(t, tc.method, w.Body.String())
		})
	}
}

func TestGroupResourceRegistersRoutesUnderPrefix(t *testing.T) {
	cases := []struct {
		name     string
		method   string
		path     string
		wantBody string
	}{
		{"New", http.MethodGet, "/api/profile/new", "New"},
		{"Create", http.MethodPost, "/api/profile", "Create"},
		{"Show", http.MethodGet, "/api/profile", "Show"},
		{"Edit", http.MethodPut, "/api/profile/edit", "Edit"},
		{"Destroy", http.MethodDelete, "/api/profile", "Destroy:"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := newTestRouter()
			g := rt.NewGroup("/api")
			var calls []string
			g.Resource("/profile", &stubResource{&calls})

			w := httptest.NewRecorder()
			r := httptest.NewRequest(tc.method, tc.path, nil)
			rt.ServeHTTP(w, r)

			require.Equal(t, http.StatusOK, w.Code)
			require.Equal(t, tc.wantBody, w.Body.String())
			require.Equal(t, []string{tc.name}, calls)
		})
	}
}

func TestGroupResourceAppliesMiddleware(t *testing.T) {
	rt := newTestRouter()
	var order []string
	var calls []string

	parent := rt.NewGroup("/api", recordMiddleware("A", &order))
	parent.Resource("/profile", &stubResource{&calls}, recordMiddleware("B", &order))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/profile", nil)
	rt.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, []string{"A", "B"}, order)
	require.Equal(t, []string{"Create"}, calls)
}

func TestGroupResourcesRegistersRoutesUnderPrefix(t *testing.T) {
	cases := []struct {
		name     string
		method   string
		path     string
		wantBody string
	}{
		{"Index", http.MethodGet, "/api/users", "Index"},
		{"New", http.MethodGet, "/api/users/new", "New"},
		{"Create", http.MethodPost, "/api/users", "Create"},
		{"Show", http.MethodGet, "/api/users/42", "Show"},
		{"Edit", http.MethodGet, "/api/users/42/edit", "Edit"},
		{"Update", http.MethodPut, "/api/users/42", "Update"},
		{"Destroy", http.MethodDelete, "/api/users/42", "Destroy:42"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := newTestRouter()
			g := rt.NewGroup("/api")
			var calls []string
			g.Resources("/users", &stubResources{stubResource{&calls}})

			w := httptest.NewRecorder()
			r := httptest.NewRequest(tc.method, tc.path, nil)
			rt.ServeHTTP(w, r)

			require.Equal(t, http.StatusOK, w.Code)
			require.Equal(t, tc.wantBody, w.Body.String())
			require.Equal(t, []string{tc.name}, calls)
		})
	}
}

func TestGroupResourcesAppliesMiddleware(t *testing.T) {
	rt := newTestRouter()
	var order []string
	var calls []string

	parent := rt.NewGroup("/api", recordMiddleware("A", &order))
	parent.Resources("/users", &stubResources{stubResource{&calls}}, recordMiddleware("B", &order))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	rt.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, []string{"A", "B"}, order)
	require.Equal(t, []string{"Index"}, calls)
}
