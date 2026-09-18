package session

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	trails "github.com/asimmons91/trails"
	"github.com/asimmons91/trails/internal/credentials"
)

const defaultCookieName = "_trails_session"

// defaultMaxCookieBytes leaves headroom under the ~4096 byte limit most
// browsers enforce per cookie, after accounting for the cookie's own
// name/attributes.
const defaultMaxCookieBytes = 4093

type config struct {
	cookieName     string
	path           string
	sameSite       http.SameSite
	secure         bool
	secureSet      bool
	maxAge         time.Duration
	maxCookieBytes int
}

// Option configures Middleware.
type Option func(*config)

// WithCookieName overrides the session cookie's name (default
// "_trails_session").
func WithCookieName(name string) Option {
	return func(c *config) { c.cookieName = name }
}

// WithPath overrides the session cookie's Path attribute (default "/").
func WithPath(path string) Option {
	return func(c *config) { c.path = path }
}

// WithSameSite overrides the session cookie's SameSite attribute (default
// http.SameSiteLaxMode).
func WithSameSite(s http.SameSite) Option {
	return func(c *config) { c.sameSite = s }
}

// WithSecure overrides whether the session cookie's Secure attribute is set.
// By default it's true everywhere except when trails.Environment() is
// "development" or "test", so local HTTP development keeps working without
// configuration.
func WithSecure(secure bool) Option {
	return func(c *config) {
		c.secure = secure
		c.secureSet = true
	}
}

// WithMaxAge sets the session cookie's Max-Age. The default, zero, omits
// Max-Age entirely, producing a browser-session cookie that's cleared when
// the browser closes.
func WithMaxAge(d time.Duration) Option {
	return func(c *config) { c.maxAge = d }
}

// WithMaxCookieBytes overrides the maximum encoded cookie value size (default
// 4093 bytes) above which Middleware logs an error and drops the Set-Cookie
// write rather than emit a cookie the browser may reject or truncate.
func WithMaxCookieBytes(n int) Option {
	return func(c *config) { c.maxCookieBytes = n }
}

const ctxKey = "trails/session"

// FromContext returns the current request's Session, as attached by
// Middleware. It is always non-nil for a request that has passed through
// Middleware.
func FromContext(c *trails.Context) *Session {
	return c.Get[*Session](ctxKey)
}

// Middleware returns cookie-store session middleware: the
// entire session is JSON-serialized and AES-256-GCM encrypted directly into
// a single cookie on write, with no server-side storage. secretKeyBase
// should be a long random secret (see trails.LoadCredentials and the
// generated app's config/credentials.go Credentials.SecretKeyBase) — anyone
// holding it can forge or read old session cookies, so treat it like any
// other credential.
//
// A cookie that fails to decrypt (missing, tampered, or encrypted with a
// different secretKeyBase — e.g. after rotation) never fails the request; it
// is treated identically to no cookie at all, and the visitor starts a
// fresh, empty session.
func Middleware(secretKeyBase string, opts ...Option) trails.MiddlewareFunc {
	cfg := &config{
		cookieName:     defaultCookieName,
		path:           "/",
		sameSite:       http.SameSiteLaxMode,
		maxCookieBytes: defaultMaxCookieBytes,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	if !cfg.secureSet {
		env := trails.Environment()
		cfg.secure = env != "development" && env != "test"
	}

	return func(next trails.HandlerFunc) trails.HandlerFunc {
		return func(c *trails.Context) error {
			sess := load(c, secretKeyBase, cfg)
			c.Set(ctxKey, sess)

			wrapped := &commitWriter{ResponseWriter: c.Response()}
			wrapped.commit = func() {
				persist(c, wrapped.ResponseWriter, sess, secretKeyBase, cfg)
			}
			c.SetResponse(wrapped)

			return next(c)
		}
	}
}

func load(c *trails.Context, secretKeyBase string, cfg *config) *Session {
	cookie, err := c.Request().Cookie(cfg.cookieName)
	if err != nil {
		return New()
	}

	plaintext, err := credentials.Decrypt(secretKeyBase, cookie.Value)
	if err != nil {
		c.Logger().Debug("session: discarding invalid session cookie", "error", err)
		return New()
	}

	var data map[string]any
	if err := json.Unmarshal(plaintext, &data); err != nil {
		c.Logger().Debug("session: discarding malformed session payload", "error", err)
		return New()
	}

	return &Session{data: data}
}

func persist(c *trails.Context, w http.ResponseWriter, sess *Session, secretKeyBase string, cfg *config) {
	if !sess.isDirty() {
		return
	}

	if sess.isEmpty() {
		http.SetCookie(w, &http.Cookie{
			Name:     cfg.cookieName,
			Value:    "",
			Path:     cfg.path,
			HttpOnly: true,
			Secure:   cfg.secure,
			SameSite: cfg.sameSite,
			MaxAge:   -1,
		})
		return
	}

	plaintext, err := json.Marshal(sess.snapshot())
	if err != nil {
		c.Logger().Error("session: marshaling session", "error", err)
		return
	}

	encrypted, err := credentials.Encrypt(secretKeyBase, plaintext)
	if err != nil {
		c.Logger().Error("session: encrypting session", "error", err)
		return
	}
	value := strings.TrimSpace(encrypted)

	if len(value) > cfg.maxCookieBytes {
		c.Logger().Error("session: session too large for a cookie, dropping Set-Cookie",
			"size", len(value), "max", cfg.maxCookieBytes)
		return
	}

	cookie := &http.Cookie{
		Name:     cfg.cookieName,
		Value:    value,
		Path:     cfg.path,
		HttpOnly: true,
		Secure:   cfg.secure,
		SameSite: cfg.sameSite,
	}
	if cfg.maxAge > 0 {
		cookie.MaxAge = int(cfg.maxAge.Seconds())
	}

	http.SetCookie(w, cookie)
}

// commitWriter defers computing/writing the session Set-Cookie header until
// the wrapped handler is about to actually flush a response — Context's
// Render/String/JSON/etc. call WriteHeader synchronously, so by the time
// code positioned after next(c) returns, headers are already sent. It is
// deliberately never "unwrapped": Context.Reset overwrites the response on
// every pooled-Context reuse, and leaving it wrapped ensures an error
// response (written later by the router's ErrorHandler) still commits the
// session.
type commitWriter struct {
	http.ResponseWriter
	committed bool
	commit    func()
}

func (w *commitWriter) WriteHeader(code int) {
	w.commitOnce()
	w.ResponseWriter.WriteHeader(code)
}

func (w *commitWriter) Write(b []byte) (int, error) {
	w.commitOnce()
	return w.ResponseWriter.Write(b)
}

func (w *commitWriter) commitOnce() {
	if w.committed {
		return
	}
	w.committed = true
	w.commit()
}
