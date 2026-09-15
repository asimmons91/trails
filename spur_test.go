package trails

import (
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/asimmons91/trails/assets"
	"github.com/asimmons91/trails/jobs"
	"github.com/stretchr/testify/require"
)

type fakeSpur struct {
	viewFS   fs.FS
	assetsFS fs.FS
	body     string
	jobKind  string
}

func (e *fakeSpur) ViewFS() fs.FS { return e.viewFS }

func (e *fakeSpur) AssetsFS() fs.FS { return e.assetsFS }

func (e *fakeSpur) Routes(g *Group) {
	g.Get("", func(c *Context) error {
		return c.String(http.StatusOK, e.body)
	})
}

func (e *fakeSpur) Jobs(r *jobs.Registry) {
	if e.jobKind != "" {
		r.Register(e.jobKind, func() jobs.Job { return &noopJob{kind: e.jobKind} })
	}
}

type noopJob struct{ kind string }

func (j *noopJob) Kind() string                      { return j.kind }
func (j *noopJob) Perform(ctx context.Context) error { return nil }

func TestMergeSpurViewsMergesHostAndSpurRoots(t *testing.T) {
	host := fstest.MapFS{"root/index.gohtml": &fstest.MapFile{Data: []byte("root")}}
	spur := &fakeSpur{viewFS: fstest.MapFS{"posts/index.gohtml": &fstest.MapFile{Data: []byte("posts")}}}

	merged := MergeSpurViews(host, Mount{Prefix: "/posts", Spur: spur})

	rootData, err := fs.ReadFile(merged, "root/index.gohtml")
	require.NoError(t, err)
	require.Equal(t, "root", string(rootData))

	postsData, err := fs.ReadFile(merged, "posts/index.gohtml")
	require.NoError(t, err)
	require.Equal(t, "posts", string(postsData))
}

func TestMergeSpurViewsHostShadowsSpurView(t *testing.T) {
	host := fstest.MapFS{"shared.gohtml": &fstest.MapFile{Data: []byte("host")}}
	spur := &fakeSpur{viewFS: fstest.MapFS{"shared.gohtml": &fstest.MapFile{Data: []byte("spur")}}}

	merged := MergeSpurViews(host, Mount{Prefix: "/spur", Spur: spur})

	data, err := fs.ReadFile(merged, "shared.gohtml")
	require.NoError(t, err)
	require.Equal(t, "host", string(data))
}

func TestMergeSpurViewsNoMountsReturnsHostOnly(t *testing.T) {
	host := fstest.MapFS{"root/index.gohtml": &fstest.MapFile{Data: []byte("root")}}

	merged := MergeSpurViews(host)

	data, err := fs.ReadFile(merged, "root/index.gohtml")
	require.NoError(t, err)
	require.Equal(t, "root", string(data))
}

func TestMergeSpurAssetsMergesHostAndSpurRoots(t *testing.T) {
	host := fstest.MapFS{"application-abc123.js": &fstest.MapFile{Data: []byte("host")}}
	spur := &fakeSpur{assetsFS: fstest.MapFS{"posts-def456.js": &fstest.MapFile{Data: []byte("posts")}}}

	merged := MergeSpurAssets(host, Mount{Prefix: "/posts", Spur: spur})

	hostData, err := fs.ReadFile(merged, "application-abc123.js")
	require.NoError(t, err)
	require.Equal(t, "host", string(hostData))

	spurData, err := fs.ReadFile(merged, "posts-def456.js")
	require.NoError(t, err)
	require.Equal(t, "posts", string(spurData))
}

func TestMergeSpurAssetsHostShadowsSpurAsset(t *testing.T) {
	host := fstest.MapFS{"manifest.json": &fstest.MapFile{Data: []byte("host")}}
	spur := &fakeSpur{assetsFS: fstest.MapFS{"manifest.json": &fstest.MapFile{Data: []byte("spur")}}}

	merged := MergeSpurAssets(host, Mount{Prefix: "/posts", Spur: spur})

	data, err := fs.ReadFile(merged, "manifest.json")
	require.NoError(t, err)
	require.Equal(t, "host", string(data))
}

func TestMergeSpurAssetsSkipsSpurWithNilAssetsFS(t *testing.T) {
	host := fstest.MapFS{"application-abc123.js": &fstest.MapFile{Data: []byte("host")}}
	spur := &fakeSpur{assetsFS: nil}

	merged := MergeSpurAssets(host, Mount{Prefix: "/posts", Spur: spur})

	data, err := fs.ReadFile(merged, "application-abc123.js")
	require.NoError(t, err)
	require.Equal(t, "host", string(data))
}

func TestMergeSpurAssetsNoMountsReturnsHostOnly(t *testing.T) {
	host := fstest.MapFS{"application-abc123.js": &fstest.MapFile{Data: []byte("host")}}

	merged := MergeSpurAssets(host)

	data, err := fs.ReadFile(merged, "application-abc123.js")
	require.NoError(t, err)
	require.Equal(t, "host", string(data))
}

func manifestFS(content string) fstest.MapFS {
	return fstest.MapFS{"manifest.json": &fstest.MapFile{Data: []byte(content)}}
}

func TestMergeSpurManifestsUnionsHostAndSpurManifests(t *testing.T) {
	host := assets.Manifest{"application.js": "application-abc123.js"}
	spur := &fakeSpur{assetsFS: manifestFS(`{"posts.js":"posts-def456.js"}`)}

	merged, err := MergeSpurManifests(host, Mount{Prefix: "/posts", Spur: spur})
	require.NoError(t, err)
	require.Equal(t, assets.Manifest{
		"application.js": "application-abc123.js",
		"posts.js":       "posts-def456.js",
	}, merged)
}

func TestMergeSpurManifestsHostWinsOnKeyConflict(t *testing.T) {
	host := assets.Manifest{"shared.js": "shared-host123.js"}
	spur := &fakeSpur{assetsFS: manifestFS(`{"shared.js":"shared-spur456.js"}`)}

	merged, err := MergeSpurManifests(host, Mount{Prefix: "/posts", Spur: spur})
	require.NoError(t, err)
	require.Equal(t, assets.Manifest{"shared.js": "shared-host123.js"}, merged)
}

func TestMergeSpurManifestsSkipsSpurWithNilAssetsFS(t *testing.T) {
	host := assets.Manifest{"application.js": "application-abc123.js"}
	spur := &fakeSpur{assetsFS: nil}

	merged, err := MergeSpurManifests(host, Mount{Prefix: "/posts", Spur: spur})
	require.NoError(t, err)
	require.Equal(t, assets.Manifest{"application.js": "application-abc123.js"}, merged)
}

func TestMergeSpurManifestsReturnsWrappedErrorWhenSpurManifestMissing(t *testing.T) {
	host := assets.Manifest{}
	spur := &fakeSpur{assetsFS: fstest.MapFS{}}

	_, err := MergeSpurManifests(host, Mount{Prefix: "/posts", Spur: spur})
	require.Error(t, err)
	require.Contains(t, err.Error(), `mounted at "/posts"`)
}

func TestRegisterSpurRoutesMountsUnderPrefix(t *testing.T) {
	rt := newTestRouter()
	spur := &fakeSpur{viewFS: fstest.MapFS{}, body: "posts-index"}

	RegisterSpurRoutes(rt, Mount{Prefix: "/posts", Spur: spur})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/posts", nil)
	rt.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "posts-index", w.Body.String())
}

func TestRegisterSpurJobsRegistersSpurJobKinds(t *testing.T) {
	reg := jobs.NewRegistry()
	spur := &fakeSpur{jobKind: "posts.notify"}

	RegisterSpurJobs(reg, Mount{Prefix: "/posts", Spur: spur})

	require.NoError(t, reg.Dispatch(context.Background(), jobs.Enqueued{Kind: "posts.notify"}))
}

func TestRegisterSpurJobsRegistersMultipleSpursIndependently(t *testing.T) {
	reg := jobs.NewRegistry()
	posts := &fakeSpur{jobKind: "posts.notify"}
	comments := &fakeSpur{jobKind: "comments.notify"}

	RegisterSpurJobs(reg,
		Mount{Prefix: "/posts", Spur: posts},
		Mount{Prefix: "/comments", Spur: comments},
	)

	require.NoError(t, reg.Dispatch(context.Background(), jobs.Enqueued{Kind: "posts.notify"}))
	require.NoError(t, reg.Dispatch(context.Background(), jobs.Enqueued{Kind: "comments.notify"}))
}

func TestRegisterSpurRoutesMountsMultipleSpursIndependently(t *testing.T) {
	rt := newTestRouter()
	posts := &fakeSpur{viewFS: fstest.MapFS{}, body: "posts-index"}
	comments := &fakeSpur{viewFS: fstest.MapFS{}, body: "comments-index"}

	RegisterSpurRoutes(rt,
		Mount{Prefix: "/posts", Spur: posts},
		Mount{Prefix: "/comments", Spur: comments},
	)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/posts", nil)
	rt.ServeHTTP(w, r)
	require.Equal(t, "posts-index", w.Body.String())

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/comments", nil)
	rt.ServeHTTP(w, r)
	require.Equal(t, "comments-index", w.Body.String())
}
