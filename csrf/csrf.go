// Package csrf provides synchronizer-token CSRF protection for trails,
// tied to a session's per-session token (see the session package). Register
// Middleware after session.Middleware, and wire PlaceholderFuncMap and
// RequestFuncMap into TrailOptions so views can render the token:
//
//	t.Use(session.Middleware(secretKeyBase), csrf.Middleware())
//
//	opts.FuncMap = csrf.PlaceholderFuncMap
//	opts.RequestFuncMap = csrf.RequestFuncMap
package csrf

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"net/http"

	trails "github.com/asimmons91/trails"
	"github.com/asimmons91/trails/session"
)

const defaultFieldName = "authenticity_token"
const defaultHeaderName = "X-CSRF-Token"

type config struct {
	fieldName  string
	headerName string
}

// Option configures Middleware and the RequestFuncMap helpers.
type Option func(*config)

// WithFieldName overrides the form field name csrfField renders and
// Middleware reads on submission (default "authenticity_token").
func WithFieldName(name string) Option {
	return func(c *config) { c.fieldName = name }
}

// WithHeaderName overrides the header name Middleware checks for a
// JS/fetch-submitted token, and that csrfMetaTag's csrf-token meta pairs
// with (default "X-CSRF-Token").
func WithHeaderName(name string) Option {
	return func(c *config) { c.headerName = name }
}

func newConfig(opts []Option) *config {
	cfg := &config{fieldName: defaultFieldName, headerName: defaultHeaderName}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

var safeMethods = map[string]bool{
	http.MethodGet:     true,
	http.MethodHead:    true,
	http.MethodOptions: true,
	http.MethodTrace:   true,
}

// Middleware verifies a synchronizer CSRF token on every unsafe-method
// request (anything but GET/HEAD/OPTIONS/TRACE) against the current
// session's token. It must be registered after session.Middleware in the
// chain; it panics if no Session is found on the Context, since that's a
// wiring bug rather than something a request can trigger.
func Middleware(opts ...Option) trails.MiddlewareFunc {
	cfg := newConfig(opts)

	return func(next trails.HandlerFunc) trails.HandlerFunc {
		return func(c *trails.Context) error {
			sess := session.FromContext(c)
			if sess == nil {
				panic("csrf: no session found on context; csrf.Middleware must be " +
					"registered after session.Middleware, e.g. " +
					"t.Use(session.Middleware(key), csrf.Middleware())")
			}

			if safeMethods[c.Request().Method] {
				return next(c)
			}

			submitted := c.Request().Header.Get(cfg.headerName)
			if submitted == "" {
				submitted = c.Request().FormValue(cfg.fieldName)
			}

			if !verify(sess.CSRFToken(), submitted) {
				return trails.NewHTTPError(http.StatusForbidden, errors.New("csrf: invalid or missing authenticity token"))
			}

			return next(c)
		}
	}
}

// mask XORs a fresh random mask over real and base64-encodes mask||masked,
// so that embedding the result in rendered HTML produces a different string
// on every render even though it decodes back to the same underlying token.
// This defends against BREACH-style compression-oracle attacks that could
// otherwise correlate a fixed reflected token's bytes to response length.
func mask(real []byte) (string, error) {
	m := make([]byte, len(real))
	if _, err := rand.Read(m); err != nil {
		return "", fmt.Errorf("csrf: reading random mask: %w", err)
	}

	masked := make([]byte, len(real))
	for i := range masked {
		masked[i] = m[i] ^ real[i]
	}

	return base64.URLEncoding.EncodeToString(append(m, masked...)), nil
}

// verify reverses mask and constant-time-compares the result against real.
func verify(real []byte, submitted string) bool {
	if submitted == "" {
		return false
	}

	raw, err := base64.URLEncoding.DecodeString(submitted)
	if err != nil || len(raw) != 2*len(real) {
		return false
	}

	m, masked := raw[:len(real)], raw[len(real):]
	unmasked := make([]byte, len(real))
	for i := range unmasked {
		unmasked[i] = m[i] ^ masked[i]
	}

	return subtle.ConstantTimeCompare(unmasked, real) == 1
}

// PlaceholderFuncMap declares csrfField/csrfMetaTag with no-op
// implementations. html/template requires every function name used in a
// template to be registered at parse time, before the real per-request
// implementation (RequestFuncMap) is available — merge this into
// TrailOptions.FuncMap at boot, and pass RequestFuncMap for the real
// behavior.
var PlaceholderFuncMap = template.FuncMap{
	"csrfField":   func() template.HTML { return "" },
	"csrfMetaTag": func() template.HTML { return "" },
}

// RequestFuncMap returns the real, per-request csrfField/csrfMetaTag
// implementations for c's session, using the default field/header names.
// Pass directly as trails.TrailOptions.RequestFuncMap; requires
// session.Middleware and Middleware to already be wired in. If Middleware
// is configured with WithFieldName/WithHeaderName, use
// NewRequestFuncMap(sameOpts...) instead so the names match.
//
// csrfField renders a hidden <input> suitable for placing inside a <form>.
// csrfMetaTag renders the csrf-param/csrf-token <meta> tags JS can read to
// send the header Middleware checks.
//
// Do not call these from a template rendered via Context.RenderBlock —
// fragments rendered that way can be cached across requests/users (see
// cache.FetchFragment), and a per-session token must never end up in a
// fragment cached for someone else.
func RequestFuncMap(c *trails.Context) template.FuncMap {
	return NewRequestFuncMap()(c)
}

// NewRequestFuncMap builds a RequestFuncMap-style function using opts,
// matching Middleware(opts...) when it's configured with WithFieldName
// and/or WithHeaderName. See RequestFuncMap for what csrfField/csrfMetaTag
// do and when not to call them.
func NewRequestFuncMap(opts ...Option) func(*trails.Context) template.FuncMap {
	return requestFuncMap(newConfig(opts))
}

func requestFuncMap(cfg *config) func(c *trails.Context) template.FuncMap {
	return func(c *trails.Context) template.FuncMap {
		token := func() (string, error) {
			sess := session.FromContext(c)
			if sess == nil {
				return "", errors.New("csrf: no session found on context")
			}
			return mask(sess.CSRFToken())
		}

		return template.FuncMap{
			"csrfField": func() (template.HTML, error) {
				t, err := token()
				if err != nil {
					return "", err
				}
				return template.HTML(fmt.Sprintf(
					`<input type="hidden" name="%s" value="%s">`,
					template.HTMLEscapeString(cfg.fieldName), t,
				)), nil
			},
			"csrfMetaTag": func() (template.HTML, error) {
				t, err := token()
				if err != nil {
					return "", err
				}
				return template.HTML(fmt.Sprintf(
					`<meta name="csrf-param" content="%s"><meta name="csrf-token" content="%s">`,
					template.HTMLEscapeString(cfg.fieldName), t,
				)), nil
			},
		}
	}
}
