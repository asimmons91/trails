// Package trustedproxy resolves the real client IP and request scheme
// (http/https) from X-Forwarded-For/X-Forwarded-Proto
// but only trusts those headers when the immediate TCP peer (r.RemoteAddr)
// is itself a configured trusted proxy. An untrusted peer's X-Forwarded-*
// headers are never consulted, since any client can send them.
//
// Unlike cors/allowedhosts, an empty trusted-proxy list does NOT make
// Middleware a no-op that skips running: it always runs, setting
// ClientIP/Scheme on every request so app code and other middleware have a
// single, always-present API for these values. With nothing configured (or
// the peer untrusted) it simply passes r.RemoteAddr/r.TLS through
// unchanged, rather than ever trusting X-Forwarded-*.
//
// There is no default list of trusted proxies (see WithTrustedProxies) —
// wire it in explicitly, e.g. behind a reverse proxy on the same private
// network:
//
//	t.Use(trustedproxy.Middleware(trustedproxy.WithTrustedProxies(trustedproxy.PrivateCIDRs...)), ...)
//
// If your proxy terminates TLS but forwards to the app over plain HTTP
// without setting X-Forwarded-Proto (or any equivalent) at all, there's
// nothing for WithTrustedProxies to verify for scheme purposes — use
// WithAssumeSSL instead:
//
//	t.Use(trustedproxy.Middleware(trustedproxy.WithTrustedProxies(trustedproxy.PrivateCIDRs...), trustedproxy.WithAssumeSSL(true)), ...)
//
// This package is independent of allowedhosts: allowedhosts validates
// r.Host directly and intentionally does not consult X-Forwarded-Host. A
// proxy that rewrites the Host header before forwarding (rather than the
// standard `proxy_set_header Host $host;`-style passthrough that preserves
// it) is an uncommon setup outside this feature's scope, so the two
// packages compose independently and can be registered in either order.
//
// As with cors/allowedhosts, requests that bypass the middleware chain
// (Router.Static) never reach this middleware.
package trustedproxy

import (
	"fmt"
	"net"
	"net/netip"
	"strings"

	trails "github.com/asimmons91/trails"
)

type config struct {
	trustedProxies []netip.Prefix
	assumeSSL      bool
}

// Option configures Middleware.
type Option func(*config)

// PrivateCIDRs is a convenience list of RFC 1918 / RFC 4193 / loopback
// private-range CIDRs, for apps that terminate TLS at a reverse proxy on the same private
// network:
//
//	trustedproxy.Middleware(trustedproxy.WithTrustedProxies(trustedproxy.PrivateCIDRs...))
//
// This is never applied automatically — see the package doc comment for
// why there is no default.
var PrivateCIDRs = []string{
	"127.0.0.1/8", "::1/128",
	"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
	"fc00::/7",
}

func parsePrefix(raw string) (netip.Prefix, error) {
	if strings.Contains(raw, "/") {
		return netip.ParsePrefix(raw)
	}
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return netip.Prefix{}, err
	}
	return netip.PrefixFrom(addr, addr.BitLen()), nil
}

// WithTrustedProxies sets the CIDR ranges (or single IPs, treated as /32 or
// /128) whose immediate TCP connection is trusted to supply accurate
// X-Forwarded-For/X-Forwarded-Proto values. The default is empty: no peer
// is ever trusted, and X-Forwarded-* is never consulted (see the package
// doc comment on why this differs from a full no-op).
//
// Panics on an unparsable entry — this runs once at startup and a bad CIDR
// is always a configuration mistake, never something a request can
// trigger, mirroring csrf.Middleware's own wiring-mistake panic.
func WithTrustedProxies(cidrs ...string) Option {
	return func(c *config) {
		for _, raw := range cidrs {
			prefix, err := parsePrefix(raw)
			if err != nil {
				panic(fmt.Sprintf("trustedproxy: invalid trusted proxy %q: %v", raw, err))
			}
			c.trustedProxies = append(c.trustedProxies, prefix)
		}
	}
}

// WithAssumeSSL forces Scheme/IsSecure to always report "https", ignoring
// both r.TLS and any X-Forwarded-Proto header.
// Use this when a proxy terminates TLS but
// forwards to the app over plain HTTP without setting any header
// indicating so, meaning there is nothing for WithTrustedProxies to
// verify: WithTrustedProxies still applies to X-Forwarded-For/ClientIP
// resolution as normal, but scheme resolution is skipped entirely. The
// default is false.
func WithAssumeSSL(assumeSSL bool) Option {
	return func(c *config) { c.assumeSSL = assumeSSL }
}

func (cfg *config) isTrusted(addr netip.Addr) bool {
	for _, p := range cfg.trustedProxies {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

const ctxKeyClientIP = "trails/trustedproxy/clientip"
const ctxKeyScheme = "trails/trustedproxy/scheme"

// ClientIP returns the current request's resolved client IP, as set by
// Middleware: the X-Forwarded-For-derived address when the peer is a
// trusted proxy, otherwise r.RemoteAddr's host part unchanged.
func ClientIP(c *trails.Context) string { return c.Get[string](ctxKeyClientIP) }

// Scheme returns "https" or "http": the X-Forwarded-Proto-derived scheme
// when the peer is a trusted proxy, otherwise "https" iff r.TLS != nil.
func Scheme(c *trails.Context) string { return c.Get[string](ctxKeyScheme) }

// IsSecure is a convenience wrapper: Scheme(c) == "https".
func IsSecure(c *trails.Context) bool { return Scheme(c) == "https" }

// peerHost strips the ":port" net/http always appends to RemoteAddr for
// HTTP/1.x and HTTP/2 requests, via net.SplitHostPort; falls back to the
// raw value if there's no port (defensive, e.g. some test/harness code sets
// RemoteAddr to a bare IP).
func peerHost(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

// forwardedAddr parses one X-Forwarded-For entry, stripping an optional
// ":port" (net.SplitHostPort handles bracketed IPv6 too) before parsing.
func forwardedAddr(entry string) (netip.Addr, error) {
	entry = strings.TrimSpace(entry)
	if host, _, err := net.SplitHostPort(entry); err == nil {
		entry = host
	}
	return netip.ParseAddr(entry)
}

// resolveClientIP walks the X-Forwarded-For chain from the right — the end
// a trusted proxy itself appends to — skipping entries that are themselves
// trusted proxies, and returns the first entry that is not.
//
// This direction is what makes the result trustworthy: every hop in a
// forwarding chain appends the address it received *before* passing the
// request on, so entries accumulate left-to-right as "oldest" to
// "newest/nearest." A malicious client fully controls its own request and
// can prepend arbitrary fake entries onto the left of X-Forwarded-For, but
// it cannot control what a trusted proxy appends on the right. Scanning
// from the right and stopping at the first entry not itself a trusted
// proxy finds the outermost hop the trusted chain actually vouches for —
// everything further right was appended by a trusted proxy, and this is
// the first entry that wasn't.
//
// A malformed entry aborts the walk entirely (returns false) rather than
// skip past it — refusing to guess is safer than silently accepting a
// header that doesn't parse as expected. Likewise, if every entry turns out
// to be a trusted proxy (an all-internal chain with no further hop to
// trust), it returns false, and the caller falls back to RemoteAddr.
func (cfg *config) resolveClientIP(xff string) (string, bool) {
	parts := strings.Split(xff, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		addr, err := forwardedAddr(parts[i])
		if err != nil {
			return "", false
		}
		if cfg.isTrusted(addr) {
			continue
		}
		return addr.String(), true
	}
	return "", false
}

// firstEntry returns the first comma-separated value in headerValue,
// trimmed. X-Forwarded-Proto is read leftmost-first — the opposite
// direction from the X-Forwarded-For walk above, since a proxy sets its own
// Proto value once at the point it terminates TLS, nearest the client, not
// appended by every subsequent hop the way X-Forwarded-For is.
func firstEntry(headerValue string) string {
	first, _, _ := strings.Cut(headerValue, ",")
	return strings.TrimSpace(first)
}

// Middleware returns middleware that resolves ClientIP/Scheme for every
// request. See the package doc comment for the trust model and the
// always-runs (non-no-op) behavior when no trusted proxies are configured.
func Middleware(opts ...Option) trails.MiddlewareFunc {
	cfg := &config{}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(next trails.HandlerFunc) trails.HandlerFunc {
		return func(c *trails.Context) error {
			r := c.Request()

			clientIP := peerHost(r.RemoteAddr)
			scheme := "http"
			if r.TLS != nil {
				scheme = "https"
			}

			peerAddr, err := netip.ParseAddr(clientIP)
			trustedPeer := err == nil && cfg.isTrusted(peerAddr)

			if trustedPeer {
				if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
					if resolved, ok := cfg.resolveClientIP(xff); ok {
						clientIP = resolved
					}
				}
				if xfp := r.Header.Get("X-Forwarded-Proto"); xfp != "" {
					if proto := firstEntry(xfp); proto != "" {
						scheme = strings.ToLower(proto)
					}
				}
			}

			if cfg.assumeSSL {
				scheme = "https"
			}

			c.Set(ctxKeyClientIP, clientIP)
			c.Set(ctxKeyScheme, scheme)

			return next(c)
		}
	}
}
