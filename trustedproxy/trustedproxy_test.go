package trustedproxy_test

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	trails "github.com/asimmons91/trails"
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

// buildCapturing registers mw and a handler that captures the resolved
// ClientIP/Scheme for the test to inspect after ServeHTTP returns.
func buildCapturing(mw trails.MiddlewareFunc) (trails.RouteBuilder, *string, *string) {
	var gotIP, gotScheme string
	build := func(r *trails.Router) {
		r.Use(mw)
		r.Get("/thing", func(c *trails.Context) error {
			gotIP = trustedproxy.ClientIP(c)
			gotScheme = trustedproxy.Scheme(c)
			return c.String(http.StatusOK, "ok")
		})
	}
	return build, &gotIP, &gotScheme
}

func TestTrustedPeerWithForwardedForResolvesClientIP(t *testing.T) {
	build, ip, _ := buildCapturing(trustedproxy.Middleware(trustedproxy.WithTrustedProxies("10.0.0.0/8")))
	trail := newTestTrail(t, build)

	req := httptest.NewRequest(http.MethodGet, "/thing", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.9")

	w := httptest.NewRecorder()
	trail.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "203.0.113.9", *ip)
}

func TestUntrustedPeerIgnoresForwardedForEntirely(t *testing.T) {
	build, ip, _ := buildCapturing(trustedproxy.Middleware(trustedproxy.WithTrustedProxies("10.0.0.0/8")))
	trail := newTestTrail(t, build)

	req := httptest.NewRequest(http.MethodGet, "/thing", nil)
	req.RemoteAddr = "198.51.100.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.9")

	w := httptest.NewRecorder()
	trail.ServeHTTP(w, req)

	require.Equal(t, "198.51.100.1", *ip)
}

func TestForwardedForChainStopsAtUntrustedEntry(t *testing.T) {
	build, ip, _ := buildCapturing(trustedproxy.Middleware(trustedproxy.WithTrustedProxies("10.0.0.0/8")))
	trail := newTestTrail(t, build)

	req := httptest.NewRequest(http.MethodGet, "/thing", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.5, 10.0.0.1")

	w := httptest.NewRecorder()
	trail.ServeHTTP(w, req)

	require.Equal(t, "203.0.113.9", *ip)
}

func TestForwardedProtoTrustedSetsHTTPSScheme(t *testing.T) {
	build, _, scheme := buildCapturing(trustedproxy.Middleware(trustedproxy.WithTrustedProxies("10.0.0.0/8")))
	trail := newTestTrail(t, build)

	req := httptest.NewRequest(http.MethodGet, "/thing", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-Proto", "https")

	w := httptest.NewRecorder()
	trail.ServeHTTP(w, req)

	require.Equal(t, "https", *scheme)
}

func TestForwardedProtoIgnoredWhenPeerUntrusted(t *testing.T) {
	build, _, scheme := buildCapturing(trustedproxy.Middleware(trustedproxy.WithTrustedProxies("10.0.0.0/8")))
	trail := newTestTrail(t, build)

	req := httptest.NewRequest(http.MethodGet, "/thing", nil)
	req.RemoteAddr = "198.51.100.1:12345"
	req.Header.Set("X-Forwarded-Proto", "https")

	w := httptest.NewRecorder()
	trail.ServeHTTP(w, req)

	require.Equal(t, "http", *scheme)
}

func TestIPv6PeerAndTrustedCIDR(t *testing.T) {
	build, ip, _ := buildCapturing(trustedproxy.Middleware(trustedproxy.WithTrustedProxies("fc00::/7")))
	trail := newTestTrail(t, build)

	req := httptest.NewRequest(http.MethodGet, "/thing", nil)
	req.RemoteAddr = "[fd00::1]:12345"
	req.Header.Set("X-Forwarded-For", "2001:db8::1")

	w := httptest.NewRecorder()
	trail.ServeHTTP(w, req)

	require.Equal(t, "2001:db8::1", *ip)
}

func TestNoTrustedProxiesConfiguredPassesThroughRemoteAddrAndTLS(t *testing.T) {
	build, ip, scheme := buildCapturing(trustedproxy.Middleware())
	trail := newTestTrail(t, build)

	req := httptest.NewRequest(http.MethodGet, "/thing", nil)
	req.RemoteAddr = "203.0.113.1:12345"
	req.TLS = &tls.ConnectionState{}
	req.Header.Set("X-Forwarded-For", "9.9.9.9")
	req.Header.Set("X-Forwarded-Proto", "http")

	w := httptest.NewRecorder()
	trail.ServeHTTP(w, req)

	require.Equal(t, "203.0.113.1", *ip)
	require.Equal(t, "https", *scheme)
}

func TestMalformedForwardedForFallsBackToRemoteAddr(t *testing.T) {
	build, ip, _ := buildCapturing(trustedproxy.Middleware(trustedproxy.WithTrustedProxies("10.0.0.0/8")))
	trail := newTestTrail(t, build)

	req := httptest.NewRequest(http.MethodGet, "/thing", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "not-an-ip")

	w := httptest.NewRecorder()
	trail.ServeHTTP(w, req)

	require.Equal(t, "10.0.0.1", *ip)
}

func TestSingleIPTrustedProxyEntryWithoutCIDRSlash(t *testing.T) {
	build, ip, _ := buildCapturing(trustedproxy.Middleware(trustedproxy.WithTrustedProxies("10.0.0.1")))
	trail := newTestTrail(t, build)

	req := httptest.NewRequest(http.MethodGet, "/thing", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.9")

	w := httptest.NewRecorder()
	trail.ServeHTTP(w, req)

	require.Equal(t, "203.0.113.9", *ip)
}

func TestInvalidTrustedProxyEntryPanics(t *testing.T) {
	require.Panics(t, func() {
		trustedproxy.Middleware(trustedproxy.WithTrustedProxies("not-a-cidr"))
	})
}

func TestAssumeSSLForcesHTTPSWithNoTLSOrHeaders(t *testing.T) {
	build, _, scheme := buildCapturing(trustedproxy.Middleware(trustedproxy.WithAssumeSSL(true)))
	trail := newTestTrail(t, build)

	req := httptest.NewRequest(http.MethodGet, "/thing", nil)
	req.RemoteAddr = "203.0.113.1:12345"

	w := httptest.NewRecorder()
	trail.ServeHTTP(w, req)

	require.Equal(t, "https", *scheme)
}

func TestAssumeSSLOverridesForwardedProtoHTTP(t *testing.T) {
	build, _, scheme := buildCapturing(trustedproxy.Middleware(
		trustedproxy.WithTrustedProxies("10.0.0.0/8"),
		trustedproxy.WithAssumeSSL(true),
	))
	trail := newTestTrail(t, build)

	req := httptest.NewRequest(http.MethodGet, "/thing", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-Proto", "http")

	w := httptest.NewRecorder()
	trail.ServeHTTP(w, req)

	require.Equal(t, "https", *scheme)
}

func TestAssumeSSLDefaultFalseLeavesSchemeUnaffected(t *testing.T) {
	build, _, scheme := buildCapturing(trustedproxy.Middleware())
	trail := newTestTrail(t, build)

	req := httptest.NewRequest(http.MethodGet, "/thing", nil)
	req.RemoteAddr = "203.0.113.1:12345"

	w := httptest.NewRecorder()
	trail.ServeHTTP(w, req)

	require.Equal(t, "http", *scheme)
}

func TestAllForwardedForEntriesTrustedFallsBackToRemoteAddr(t *testing.T) {
	build, ip, _ := buildCapturing(trustedproxy.Middleware(trustedproxy.WithTrustedProxies("10.0.0.0/8")))
	trail := newTestTrail(t, build)

	req := httptest.NewRequest(http.MethodGet, "/thing", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "10.0.0.5, 10.0.0.2")

	w := httptest.NewRecorder()
	trail.ServeHTTP(w, req)

	require.Equal(t, "10.0.0.1", *ip)
}
