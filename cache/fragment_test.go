package cache_test

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	trails "github.com/asimmons91/trails"
	"github.com/asimmons91/trails/cache"
	"github.com/asimmons91/trails/cache/backend/memory"
	"github.com/stretchr/testify/require"
)

type fragmentPageData struct {
	Ctx     *trails.Context
	Key     string
	Product struct{ Name string }
}

func newFragmentTestTrail(t *testing.T, store cache.Store) *trails.Trail {
	t.Helper()

	viewFS := fstest.MapFS{
		"layouts/application.gohtml": &fstest.MapFile{Data: []byte(
			`{{define "application"}}{{template "content" .}}{{end}}`,
		)},
		"products/_card.gohtml": &fstest.MapFile{Data: []byte(
			`{{define "card"}}card:{{.Name}}{{end}}`,
		)},
		"products/show.gohtml": &fstest.MapFile{Data: []byte(
			`{{define "content"}}{{cached .Ctx "card" .Key .Product}}{{end}}`,
		)},
	}

	funcMap := template.FuncMap{
		"cached": func(ctx *trails.Context, block, key string, data any) (template.HTML, error) {
			return cache.FetchFragment(ctx.Request().Context(), ctx, store, block, key, time.Minute, data)
		},
	}

	trail, err := trails.New(trails.WithDefaultOptions(&trails.TrailOptions{
		ViewFS:  viewFS,
		FuncMap: funcMap,
		AssetsFS: fstest.MapFS{
			"manifest.json": &fstest.MapFile{Data: []byte("{}")},
		},
		ConfigFS: fstest.MapFS{
			"importmap.toml": &fstest.MapFile{Data: []byte("")},
		},
		RouteBuilder: func(r *trails.Router) {
			r.Get("/show", func(c *trails.Context) error {
				data := fragmentPageData{Ctx: c, Key: c.Request().URL.Query().Get("key")}
				data.Product.Name = c.Request().URL.Query().Get("name")
				return c.Render(http.StatusOK, "products/show", data)
			})
		},
	}))
	require.NoError(t, err)

	return trail
}

func TestFetchFragmentCachesAcrossRequests(t *testing.T) {
	store := memory.New()
	trail := newFragmentTestTrail(t, store)

	w1 := httptest.NewRecorder()
	trail.ServeHTTP(w1, httptest.NewRequest(http.MethodGet, "/show?key=k1&name=Ada", nil))
	require.Equal(t, "card:Ada", w1.Body.String())

	// Same key, different name: should still return the cached "Ada" render.
	w2 := httptest.NewRecorder()
	trail.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/show?key=k1&name=Grace", nil))
	require.Equal(t, "card:Ada", w2.Body.String())
}

func TestFetchFragmentMissesOnDifferentKey(t *testing.T) {
	store := memory.New()
	trail := newFragmentTestTrail(t, store)

	w1 := httptest.NewRecorder()
	trail.ServeHTTP(w1, httptest.NewRequest(http.MethodGet, "/show?key=k1&name=Ada", nil))
	require.Equal(t, "card:Ada", w1.Body.String())

	w2 := httptest.NewRecorder()
	trail.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/show?key=k2&name=Grace", nil))
	require.Equal(t, "card:Grace", w2.Body.String())
}
