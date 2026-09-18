// Package secureheaders sets static, response-wide security headers
// Register it with Router.Use/Trail.Use;
// it applies to every response and has no dependency on session or csrf, so
// registering it outermost (first in the middleware list) ensures the
// headers land even on error responses:
//
//	t.Use(secureheaders.Middleware(), session.Middleware(secretKeyBase), csrf.Middleware())
package secureheaders

import (
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	trails "github.com/asimmons91/trails"
)

// FrameOptions is a value for the X-Frame-Options header.
type FrameOptions string

const (
	FrameOptionsDeny       FrameOptions = "DENY"
	FrameOptionsSameOrigin FrameOptions = "SAMEORIGIN"
)

// defaultHSTSMaxAge is two years.
const defaultHSTSMaxAge = 2 * 365 * 24 * time.Hour

const defaultReferrerPolicy = "strict-origin-when-cross-origin"

type config struct {
	hstsMaxAge            time.Duration
	hstsIncludeSubDomains bool
	hstsPreload           bool
	contentTypeNosniff    bool
	frameOptions          FrameOptions
	referrerPolicy        string
	csp                   map[string][]string
}

// Option configures Middleware.
type Option func(*config)

// WithHSTSMaxAge overrides the Strict-Transport-Security max-age (default
// two years). A value <= 0 omits the header entirely.
//
// Strict-Transport-Security is ignored by browsers unless the response was
// received over an already-secure connection (RFC 6797), so leaving it on
// by default cannot break a plain-HTTP local development server the way
// some of Django's other SECURE_* settings can.
func WithHSTSMaxAge(d time.Duration) Option {
	return func(c *config) { c.hstsMaxAge = d }
}

// WithHSTSIncludeSubDomains sets whether Strict-Transport-Security includes
// the includeSubDomains directive (default true).
func WithHSTSIncludeSubDomains(b bool) Option {
	return func(c *config) { c.hstsIncludeSubDomains = b }
}

// WithHSTSPreload sets whether Strict-Transport-Security includes the
// preload directive (default false — submitting a domain to browsers'
// preload lists is a deliberate, hard-to-reverse action, not a safe
// default).
func WithHSTSPreload(b bool) Option {
	return func(c *config) { c.hstsPreload = b }
}

// WithContentTypeNosniff sets whether X-Content-Type-Options: nosniff is
// sent (default true).
func WithContentTypeNosniff(b bool) Option {
	return func(c *config) { c.contentTypeNosniff = b }
}

// WithFrameOptions overrides the X-Frame-Options header (default
// FrameOptionsDeny). An empty value omits the header.
func WithFrameOptions(o FrameOptions) Option {
	return func(c *config) { c.frameOptions = o }
}

// WithReferrerPolicy overrides the Referrer-Policy header (default
// "strict-origin-when-cross-origin"). An empty value omits the header.
func WithReferrerPolicy(p string) Option {
	return func(c *config) { c.referrerPolicy = p }
}

// WithCSP sets the Content-Security-Policy header from a directive name to
// source-list map, e.g. WithCSP(map[string][]string{"default-src":
// {"'self'"}, "img-src": {"'self'", "data:"}}). Directives are joined with
// "; " in a deterministic (sorted by directive name) order. A nil or empty
// map omits the header entirely, which is the default: a safe default CSP
// can't be guessed — it depends on what the application actually loads.
func WithCSP(directives map[string][]string) Option {
	return func(c *config) { c.csp = directives }
}

func newConfig(opts []Option) *config {
	cfg := &config{
		hstsMaxAge:            defaultHSTSMaxAge,
		hstsIncludeSubDomains: true,
		contentTypeNosniff:    true,
		frameOptions:          FrameOptionsDeny,
		referrerPolicy:        defaultReferrerPolicy,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

func hstsValue(cfg *config) string {
	var b strings.Builder
	b.WriteString("max-age=")
	b.WriteString(strconv.Itoa(int(cfg.hstsMaxAge.Seconds())))
	if cfg.hstsIncludeSubDomains {
		b.WriteString("; includeSubDomains")
	}
	if cfg.hstsPreload {
		b.WriteString("; preload")
	}
	return b.String()
}

func cspValue(directives map[string][]string) string {
	names := slices.Sorted(maps.Keys(directives))

	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+" "+strings.Join(directives[name], " "))
	}
	return strings.Join(parts, "; ")
}

// headerSet is the fixed set of header name/value pairs Middleware writes on
// every response, computed once since none of it varies by request.
type headerSet [][2]string

func buildHeaders(cfg *config) headerSet {
	var hs headerSet

	if cfg.hstsMaxAge > 0 {
		hs = append(hs, [2]string{"Strict-Transport-Security", hstsValue(cfg)})
	}
	if cfg.contentTypeNosniff {
		hs = append(hs, [2]string{"X-Content-Type-Options", "nosniff"})
	}
	if cfg.frameOptions != "" {
		hs = append(hs, [2]string{"X-Frame-Options", string(cfg.frameOptions)})
	}
	if cfg.referrerPolicy != "" {
		hs = append(hs, [2]string{"Referrer-Policy", cfg.referrerPolicy})
	}
	if len(cfg.csp) > 0 {
		hs = append(hs, [2]string{"Content-Security-Policy", cspValue(cfg.csp)})
	}

	return hs
}

// Middleware returns middleware that sets HSTS, X-Content-Type-Options,
// X-Frame-Options, Referrer-Policy, and (if configured) Content-Security-Policy
// headers on every response, using secure-by-default values unless overridden
// by Option. See the package doc comment for placement in the middleware
// chain.
func Middleware(opts ...Option) trails.MiddlewareFunc {
	hs := buildHeaders(newConfig(opts))

	return func(next trails.HandlerFunc) trails.HandlerFunc {
		return func(c *trails.Context) error {
			header := c.Response().Header()
			for _, kv := range hs {
				header.Set(kv[0], kv[1])
			}
			return next(c)
		}
	}
}
