package allowedhosts_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	trails "github.com/asimmons91/trails"
	"github.com/asimmons91/trails/allowedhosts"
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
		r.Get("/thing", func(c *trails.Context) error {
			calls++
			return c.String(http.StatusOK, "ok")
		})
	}
	return build, &calls
}

func doRequest(trail *trails.Trail, host string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/thing", nil)
	req.Host = host

	w := httptest.NewRecorder()
	trail.ServeHTTP(w, req)
	return w
}

func TestExactHostMatchIsAllowed(t *testing.T) {
	build, _ := buildWithHandlerCounter(allowedhosts.Middleware(allowedhosts.WithAllowedHosts("example.com")))
	trail := newTestTrail(t, build)

	w := doRequest(trail, "example.com")

	require.Equal(t, http.StatusOK, w.Code)
}

func TestSubdomainWildcardMatchesRootDomain(t *testing.T) {
	build, _ := buildWithHandlerCounter(allowedhosts.Middleware(allowedhosts.WithAllowedHosts(".example.com")))
	trail := newTestTrail(t, build)

	w := doRequest(trail, "example.com")

	require.Equal(t, http.StatusOK, w.Code)
}

func TestSubdomainWildcardMatchesSubdomain(t *testing.T) {
	build, _ := buildWithHandlerCounter(allowedhosts.Middleware(allowedhosts.WithAllowedHosts(".example.com")))
	trail := newTestTrail(t, build)

	require.Equal(t, http.StatusOK, doRequest(trail, "www.example.com").Code)
	require.Equal(t, http.StatusOK, doRequest(trail, "a.b.example.com").Code)
}

func TestSubdomainWildcardRejectsUnrelatedDomain(t *testing.T) {
	build, calls := buildWithHandlerCounter(allowedhosts.Middleware(allowedhosts.WithAllowedHosts(".example.com")))
	trail := newTestTrail(t, build)

	w := doRequest(trail, "evilexample.com")

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Equal(t, 0, *calls)
}

func TestWildcardStarAllowsAnyHost(t *testing.T) {
	build, _ := buildWithHandlerCounter(allowedhosts.Middleware(allowedhosts.WithAllowedHosts("*")))
	trail := newTestTrail(t, build)

	require.Equal(t, http.StatusOK, doRequest(trail, "anything.example.org").Code)
}

func TestPortIsStrippedBeforeMatching(t *testing.T) {
	build, _ := buildWithHandlerCounter(allowedhosts.Middleware(allowedhosts.WithAllowedHosts("example.com")))
	trail := newTestTrail(t, build)

	w := doRequest(trail, "example.com:8080")

	require.Equal(t, http.StatusOK, w.Code)
}

func TestIPv6HostWithPortIsHandled(t *testing.T) {
	build, _ := buildWithHandlerCounter(allowedhosts.Middleware(allowedhosts.WithAllowedHosts("::1")))
	trail := newTestTrail(t, build)

	w := doRequest(trail, "[::1]:8080")

	require.Equal(t, http.StatusOK, w.Code)
}

func TestMatchingIsCaseInsensitive(t *testing.T) {
	build, _ := buildWithHandlerCounter(allowedhosts.Middleware(allowedhosts.WithAllowedHosts("Example.COM")))
	trail := newTestTrail(t, build)

	w := doRequest(trail, "example.com")

	require.Equal(t, http.StatusOK, w.Code)
}

func TestEmptyAllowedHostsIsNoOp(t *testing.T) {
	build, _ := buildWithHandlerCounter(allowedhosts.Middleware())
	trail := newTestTrail(t, build)

	w := doRequest(trail, "anything.example.org")

	require.Equal(t, http.StatusOK, w.Code)
}

func TestMismatchedHostReturnsBadRequest(t *testing.T) {
	build, calls := buildWithHandlerCounter(allowedhosts.Middleware(allowedhosts.WithAllowedHosts("example.com")))
	trail := newTestTrail(t, build)

	w := doRequest(trail, "attacker.example")

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Equal(t, 0, *calls)
}
