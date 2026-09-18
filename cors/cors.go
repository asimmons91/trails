// Package cors provides cross-origin resource sharing support.
//
// Register it with Router.Use/Trail.Use or scoped to a Group as usual, but
// because trails registers routes against Go's http.ServeMux using
// per-method patterns ("GET /path"), an incoming OPTIONS preflight request
// for a path that only has GET/POST/etc. registered never reaches any
// middleware at all — ServeMux answers 405 itself before any handler runs.
// You must also register a matching OPTIONS route, wrapped by the same
// cors.Middleware, for every prefix preflight needs to work on:
//
//	api := r.NewGroup("/api", cors.Middleware(cors.WithAllowedOrigins("https://app.example.com")))
//	api.HandleFunc(http.MethodOptions, "/{path...}", cors.PreflightHandler)
//
// Do not register "OPTIONS /{path...}" at the router root in an app that
// also uses Router.Static: Go's ServeMux treats a method-specific root
// wildcard and a method-less static prefix (e.g. "/assets/") as
// conflicting patterns and panics at startup. Scope any OPTIONS wildcard to
// your API's own prefix instead.
//
// A cors.Middleware scoped to a Group runs inside (after) any middleware
// already registered on the Router via Use — group middleware always nests
// inside router-global middleware, regardless of the order the two are
// set up in. That's harmless to put ahead of cors.Middleware here: csrf's
// safe-method allowlist already includes OPTIONS, and session.Middleware
// never blocks a request, so a preflight simply passes through them before
// reaching cors.Middleware's own short-circuit.
package cors

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	trails "github.com/asimmons91/trails"
)

var defaultAllowedMethods = []string{
	http.MethodGet, http.MethodHead, http.MethodPost,
	http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions,
}

var defaultAllowedHeaders = []string{"Accept", "Content-Type", "Authorization"}

type config struct {
	allowedOrigins   []string
	allowAnyOrigin   bool
	allowedMethods   []string
	allowedHeaders   []string
	allowAnyHeader   bool
	exposedHeaders   []string
	allowCredentials bool
	maxAge           time.Duration
}

// Option configures Middleware.
type Option func(*config)

// WithAllowedOrigins sets the origins allowed to make cross-origin
// requests. Pass "*" to allow any origin. The default is empty, which
// makes Middleware a no-op: no CORS headers are ever set, identical to not
// using this middleware at all.
func WithAllowedOrigins(origins ...string) Option {
	return func(c *config) {
		c.allowedOrigins = origins
		c.allowAnyOrigin = slicesContains(origins, "*")
	}
}

// WithAllowedMethods overrides the methods advertised in
// Access-Control-Allow-Methods on a preflight response (default GET, HEAD,
// POST, PUT, PATCH, DELETE, OPTIONS).
func WithAllowedMethods(methods ...string) Option {
	return func(c *config) { c.allowedMethods = methods }
}

// WithAllowedHeaders overrides the request headers advertised in
// Access-Control-Allow-Headers on a preflight response (default Accept,
// Content-Type, Authorization). Pass "*" to echo back whatever the request
// asked for in Access-Control-Request-Headers verbatim.
func WithAllowedHeaders(headers ...string) Option {
	return func(c *config) {
		c.allowedHeaders = headers
		c.allowAnyHeader = slicesContains(headers, "*")
	}
}

// WithExposedHeaders sets the response headers, beyond the CORS-safelisted
// set, that Access-Control-Expose-Headers grants scripts on the requesting
// origin permission to read.
func WithExposedHeaders(headers ...string) Option {
	return func(c *config) { c.exposedHeaders = headers }
}

// WithAllowCredentials sets whether Access-Control-Allow-Credentials: true
// is sent, permitting cookies/HTTP auth on the cross-origin request
// (default false). When true and the matched allowed origin is "*",
// Middleware echoes the request's literal Origin instead of "*", since the
// Fetch spec forbids a literal wildcard on a credentialed response.
func WithAllowCredentials(b bool) Option {
	return func(c *config) { c.allowCredentials = b }
}

// WithMaxAge sets Access-Control-Max-Age, letting the browser cache a
// preflight response for the given duration. The default, zero, omits the
// header and leaves caching to the browser's own default.
func WithMaxAge(d time.Duration) Option {
	return func(c *config) { c.maxAge = d }
}

func newConfig(opts []Option) *config {
	cfg := &config{
		allowedMethods: defaultAllowedMethods,
		allowedHeaders: defaultAllowedHeaders,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

func slicesContains(ss []string, s string) bool {
	return slices.Contains(ss, s)
}

func (cfg *config) matchOrigin(origin string) bool {
	if origin == "" {
		return false
	}
	if cfg.allowAnyOrigin {
		return true
	}
	return slicesContains(cfg.allowedOrigins, origin)
}

// allowOriginValue returns the value to send as Access-Control-Allow-Origin
// for a matched request origin.
func (cfg *config) allowOriginValue(origin string) string {
	if cfg.allowAnyOrigin && !cfg.allowCredentials {
		return "*"
	}
	return origin
}

// isPreflight reports whether r is a CORS preflight request. A bare OPTIONS
// request without Access-Control-Request-Method is not a CORS preflight and
// is left for the handler chain to deal with as it would any other request.
func isPreflight(r *http.Request) bool {
	return r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != ""
}

// PreflightHandler is a no-op handler to register at "OPTIONS <pattern>"
// alongside Middleware, purely so http.ServeMux has a matching method and
// pattern to route a preflight request to at all. Middleware itself writes
// the complete preflight response (status and headers) before
// PreflightHandler's body would ever run.
func PreflightHandler(c *trails.Context) error { return nil }

// Middleware returns CORS middleware configured by opts. See the package
// doc comment for the ServeMux/PreflightHandler registration this requires
// for preflight requests to work.
func Middleware(opts ...Option) trails.MiddlewareFunc {
	cfg := newConfig(opts)
	enabled := len(cfg.allowedOrigins) > 0

	return func(next trails.HandlerFunc) trails.HandlerFunc {
		return func(c *trails.Context) error {
			// With no allowed origins configured, Middleware is a total
			// no-op — including leaving preflight requests to fall through
			// to next(c) (typically PreflightHandler) untouched, rather
			// than short-circuiting a request nothing here was set up to
			// handle.
			if !enabled {
				return next(c)
			}

			r := c.Request()
			origin := r.Header.Get("Origin")
			matched := cfg.matchOrigin(origin)

			if isPreflight(r) {
				header := c.Response().Header()
				header.Add("Vary", "Origin")
				header.Add("Vary", "Access-Control-Request-Method")
				header.Add("Vary", "Access-Control-Request-Headers")

				// Preflight always answers 204, matched or not: CORS is a
				// browser-enforced relaxation, not a server-side security
				// boundary, and varying the status code by origin would let
				// a response be used to enumerate allowed origins. A
				// non-matching origin simply gets none of the
				// Access-Control-Allow-* headers below, so the browser
				// still blocks the actual request itself.
				if matched {
					header.Set("Access-Control-Allow-Origin", cfg.allowOriginValue(origin))
					if cfg.allowCredentials {
						header.Set("Access-Control-Allow-Credentials", "true")
					}
					header.Set("Access-Control-Allow-Methods", strings.Join(cfg.allowedMethods, ", "))
					if cfg.allowAnyHeader {
						if reqHeaders := r.Header.Get("Access-Control-Request-Headers"); reqHeaders != "" {
							header.Set("Access-Control-Allow-Headers", reqHeaders)
						}
					} else {
						header.Set("Access-Control-Allow-Headers", strings.Join(cfg.allowedHeaders, ", "))
					}
					if cfg.maxAge > 0 {
						header.Set("Access-Control-Max-Age", strconv.Itoa(int(cfg.maxAge.Seconds())))
					}
				}

				c.Response().WriteHeader(http.StatusNoContent)
				return nil
			}

			if !matched {
				return next(c)
			}

			header := c.Response().Header()
			header.Add("Vary", "Origin")
			header.Set("Access-Control-Allow-Origin", cfg.allowOriginValue(origin))
			if cfg.allowCredentials {
				header.Set("Access-Control-Allow-Credentials", "true")
			}
			if len(cfg.exposedHeaders) > 0 {
				header.Set("Access-Control-Expose-Headers", strings.Join(cfg.exposedHeaders, ", "))
			}

			return next(c)
		}
	}
}
