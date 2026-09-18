// Package ratelimit throttles requests per client using a cache.Store-backed
// counter implemented as ordinary trails middleware.
// Each request increments a fixed-window counter keyed by HTTP method
// + path + a per-client identity (see WithKeyFunc); once the configured
// limit is exceeded within the configured period, the request is rejected
// with 429 Too Many Requests and a Retry-After header (or, if
// WithOnLimited is set, whatever response that callback produces instead).
//
// The counter's atomicity comes entirely from cache.Store.Increment, so any
// Store works: cache/backend/memory for a single process, or
// cache/backend/database (trails' equivalent of rate limiting against a
// SolidCache-style database-backed store) to share limits across multiple
// app processes.
//
// The default per-client identity is trustedproxy.ClientIP, so register
// Middleware after trustedproxy.Middleware if you're behind a reverse
// proxy:
//
//	t.Use(trustedproxy.Middleware(trustedproxy.WithTrustedProxies(...)), ratelimit.Middleware(store, 100))
//
// This is not a hard requirement the way csrf.Middleware's session
// dependency is — trustedproxy.Middleware not being wired in is not a
// wiring mistake ratelimit can detect or needs to reject; it just means
// WithKeyFunc's default falls back to the raw TCP peer address
// (r.RemoteAddr), which is still a valid per-client identity, just one an
// upstream proxy could make inaccurate (every client behind the same proxy
// would share one bucket). Unlike csrf's panic-on-missing-dependency, this
// degrades gracefully because there's no security property being silently
// bypassed — only a coarser default grouping.
package ratelimit

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	trails "github.com/asimmons91/trails"
	"github.com/asimmons91/trails/cache"
	"github.com/asimmons91/trails/trustedproxy"
)

const defaultPeriod = time.Minute

// KeyFunc computes the cache key identifying which bucket a request counts
// against.
type KeyFunc func(c *trails.Context) string

// OnLimitedFunc fully replaces the built-in 429 response when a client has
// exceeded its.
// retryAfter is how long remains in the current window.
type OnLimitedFunc func(c *trails.Context, retryAfter time.Duration) error

type config struct {
	period    time.Duration
	keyFunc   KeyFunc
	name      string
	onLimited OnLimitedFunc
}

// Option configures Middleware.
type Option func(*config)

// WithPeriod overrides the fixed window a limit applies over (default one
// minute).
func WithPeriod(d time.Duration) Option {
	return func(c *config) { c.period = d }
}

// WithKeyFunc overrides how a request's bucket is identified (default
// defaultKeyFunc: HTTP method + path + resolved client identity). Note
// WithName composes additively with whichever KeyFunc is in effect, rather
// than replacing it.
func WithKeyFunc(fn KeyFunc) Option {
	return func(c *config) { c.keyFunc = fn }
}

// WithName adds a fixed disambiguator to every key this Middleware
// computes, so stacking two Middleware calls on the same route (e.g. a
// tight per-second limit and a looser per-day one) doesn't collide on the
// same counter. It composes additively with KeyFunc — per-client identity
// still varies the key independently under the same name — rather than
// replacing it.
func WithName(name string) Option {
	return func(c *config) { c.name = name }
}

// WithOnLimited fully replaces the built-in 429 + Retry-After response
// with fn when a client exceeds its limit.
// The default (no WithOnLimited) sets a Retry-After
// header and returns a 429 HTTPError.
func WithOnLimited(fn OnLimitedFunc) Option {
	return func(c *config) { c.onLimited = fn }
}

func newConfig(opts []Option) *config {
	cfg := &config{period: defaultPeriod, keyFunc: defaultKeyFunc}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// defaultKeyFunc buckets by HTTP method + URL path (no query string — an
// arbitrary query string must not create a fresh bucket) + the resolved
// per-client identity.
func defaultKeyFunc(c *trails.Context) string {
	r := c.Request()
	return r.Method + " " + r.URL.Path + " " + identity(c)
}

// identity resolves the per-client identity a request counts against:
// trustedproxy.ClientIP when trustedproxy.Middleware is wired in (it
// returns "" only when that middleware never ran), else the raw TCP peer
// address.
func identity(c *trails.Context) string {
	if ip := trustedproxy.ClientIP(c); ip != "" {
		return ip
	}
	return peerHost(c.Request().RemoteAddr)
}

// peerHost mirrors trustedproxy's own unexported peerHost helper, stripping
// the ":port" net/http always appends to RemoteAddr. Re-derived locally
// rather than exporting trustedproxy's private helper for this one
// downstream caller.
func peerHost(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

func buildKey(cfg *config, c *trails.Context) string {
	key := cfg.keyFunc(c)
	if cfg.name != "" {
		key = cfg.name + " " + key
	}
	return key
}

// Middleware returns middleware that rejects requests once a client has
// made more than limit requests within the configured period (see
// WithPeriod), counted atomically via store.Increment. See the package doc
// comment for placement in the middleware chain and defaults.
func Middleware(store cache.Store, limit int64, opts ...Option) trails.MiddlewareFunc {
	cfg := newConfig(opts)

	return func(next trails.HandlerFunc) trails.HandlerFunc {
		return func(c *trails.Context) error {
			key := buildKey(cfg, c)

			count, expiresAt, err := store.Increment(c.Request().Context(), key, 1, cfg.period)
			if err != nil {
				return err
			}

			if count <= limit {
				return next(c)
			}

			retryAfter := time.Until(expiresAt)
			if retryAfter < 0 {
				retryAfter = 0
			}

			if cfg.onLimited != nil {
				return cfg.onLimited(c, retryAfter)
			}

			c.Response().Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
			return trails.NewHTTPError(http.StatusTooManyRequests,
				fmt.Errorf("ratelimit: exceeded %d requests per %s", limit, cfg.period))
		}
	}
}
