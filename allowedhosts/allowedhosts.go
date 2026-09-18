// Package allowedhosts validates the incoming Host header against a
// configured allowlist before any route handler runs — trails' equivalent
// of Django's ALLOWED_HOSTS setting / Rails' config.hosts
// (ActionDispatch::HostAuthorization). It defends against Host-header
// attacks (cache poisoning, password-reset-link poisoning, and any code
// that builds absolute URLs from the request) that rely on the application
// trusting an attacker-controlled Host header.
//
// Register it first/outermost, ahead of secureheaders and everything else,
// so a disallowed Host is rejected before any other middleware does work:
//
//	t.Use(
//	    allowedhosts.Middleware(allowedhosts.WithAllowedHosts("example.com", ".example.com")),
//	    secureheaders.Middleware(), session.Middleware(secretKeyBase), csrf.Middleware(),
//	)
//
// Like every other trails middleware, this only runs for routes registered
// through Router's HandleFunc-based helpers (Get/Post/NewGroup/etc.).
// Router.Static bypasses the middleware chain entirely (see the cors
// package doc comment for the same caveat) — static file routes are never
// Host-checked.
package allowedhosts

import (
	"fmt"
	"net"
	"net/http"
	"slices"
	"strings"

	trails "github.com/asimmons91/trails"
)

type config struct {
	hosts    []string
	allowAny bool
}

// Option configures Middleware.
type Option func(*config)

// WithAllowedHosts sets the hostnames permitted in the Host header. Each
// entry is matched case-insensitively against the Host header with any
// ":port" stripped first. An entry beginning with "." (e.g.
// ".example.com") matches that domain itself and any subdomain
// (www.example.com, a.b.example.com), matching Django's ALLOWED_HOSTS
// semantics exactly. Pass "*" to allow any host, mirroring cors's own "*"
// convention for WithAllowedOrigins.
//
// The default is empty, which makes Middleware a total no-op — no Host
// header is ever checked, identical to not using this middleware at all.
// Unlike a hard failure, this lets the middleware be wired in
// unconditionally (e.g. in a generated app template) and only take effect
// once a real allowlist is configured; it is not a safe production
// default, so set this explicitly before deploying.
func WithAllowedHosts(hosts ...string) Option {
	return func(c *config) {
		c.hosts = hosts
		c.allowAny = slices.Contains(hosts, "*")
	}
}

func newConfig(opts []Option) *config {
	cfg := &config{}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// stripPort removes a trailing ":port" from a Host header value, handling
// bracketed IPv6 ("[::1]:8080") via net.SplitHostPort. A value with no port
// (net.SplitHostPort's "missing port" error) is returned unchanged.
func stripPort(hostport string) string {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		return hostport
	}
	return host
}

func (cfg *config) matches(host string) bool {
	if cfg.allowAny {
		return true
	}

	host = strings.ToLower(host)
	for _, h := range cfg.hosts {
		h = strings.ToLower(h)
		if strings.HasPrefix(h, ".") {
			if host == h[1:] || strings.HasSuffix(host, h) {
				return true
			}
			continue
		}
		if host == h {
			return true
		}
	}
	return false
}

// Middleware returns middleware that rejects any request whose Host header
// doesn't match the configured allowlist with
// trails.NewHTTPError(http.StatusBadRequest, ...) (matching Django's 400
// DisallowedHost / Rails' default blocked-host response). See the package
// doc comment for the empty-list no-op default and Router.Static caveat.
func Middleware(opts ...Option) trails.MiddlewareFunc {
	cfg := newConfig(opts)
	enabled := len(cfg.hosts) > 0

	return func(next trails.HandlerFunc) trails.HandlerFunc {
		return func(c *trails.Context) error {
			if !enabled {
				return next(c)
			}

			host := stripPort(c.Request().Host)
			if !cfg.matches(host) {
				return trails.NewHTTPError(http.StatusBadRequest,
					fmt.Errorf("allowedhosts: host %q is not permitted", host))
			}

			return next(c)
		}
	}
}
