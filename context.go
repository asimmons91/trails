package trails

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"sync"
	"unsafe"
)

const charUtf8 = "charset=UTF-8"
const textPlain = "text/plain"
const textHtml = "text/html"
const applicationJson = "application/json"
const textPlainUtf8 = textPlain + "; " + charUtf8
const textHtmlUtf8 = textHtml + "; " + charUtf8

func stringToBytes(s string) []byte {
	if s == "" {
		return nil
	}
	return unsafe.Slice(unsafe.StringData(s), len(s))
}

// Context is the per-request handle a HandlerFunc/MiddlewareFunc
// receives: it wraps the underlying http.Request/ResponseWriter, carries
// a request-scoped key/value store (Get/Set), and provides response
// helpers (String/HTML/JSON/Blob/Render). One Context is reused across
// requests via a sync.Pool — see Reset.
type Context struct {
	trail    *Trail
	store    map[string]any
	lock     sync.RWMutex
	request  *http.Request
	response http.ResponseWriter
	action   string
}

func newContext(w http.ResponseWriter, r *http.Request, t *Trail) *Context {
	return &Context{
		request:  r,
		response: w,
		trail:    t,
		lock:     sync.RWMutex{},
	}
}

func (c *Context) writeContentType(contentType string) {
	header := c.response.Header()
	if header.Get("Content-Type") == "" {
		header.Set("Content-Type", contentType)
	}
}

// Reset rebinds c to a new request/response pair and clears its
// key/value store and action, for reuse from Router's Context pool. App
// code never needs to call this itself.
func (c *Context) Reset(w http.ResponseWriter, r *http.Request) {
	c.request = r
	c.response = w
	c.action = ""
	clear(c.store)
}

// Get returns the value stored at key via Set, asserted to type T. It
// returns T's zero value both when key was never Set and when the stored
// value isn't assignable to T — there is no way to distinguish the two
// from the return value alone.
func (c *Context) Get[T any](key string) T {
	c.lock.RLock()
	v := c.store[key]
	c.lock.RUnlock()

	t, _ := v.(T)
	return t
}

// Set stores val at key in c's request-scoped key/value store, for
// retrieval via Get. It is safe for concurrent use.
func (c *Context) Set(key string, val any) {
	c.lock.Lock()
	if c.store == nil {
		c.store = make(map[string]any)
	}
	c.store[key] = val
	c.lock.Unlock()
}

// Request returns the underlying *http.Request.
func (c *Context) Request() *http.Request { return c.request }

// Response returns the underlying http.ResponseWriter — or, after
// SetResponse, whatever writer was swapped in.
func (c *Context) Response() http.ResponseWriter { return c.response }

// SetResponse swaps the ResponseWriter a handler writes to — every other
// write on c (Blob, String, HTML, JSON, Render) goes through whatever
// writer is current. Middleware uses this to interpose a recording or
// buffering writer around the rest of the chain (see cache/httpcache).
func (c *Context) SetResponse(w http.ResponseWriter) { c.response = w }

// Logger returns the Trail's configured logger (TrailOptions.Logger).
func (c *Context) Logger() *slog.Logger { return c.trail.logger }

// Context returns the Trail's own long-lived context.Context
// (TrailOptions.Context) — not a per-request context; use
// Request().Context() for that.
func (c *Context) Context() context.Context { return c.trail.context }

// Bind populates target from the request via the Trail's configured
// Binder (TrailOptions.Binder, DefaultBinder unless overridden) — see
// DefaultBinder.Bind for the default's path/query/body precedence.
func (c *Context) Bind(target any) error {
	return c.trail.binder.Bind(c, target)
}

// Blob writes b as the response body with status code, setting
// Content-Type to contentType only if it isn't already set (so a
// handler or middleware that pre-set it wins).
func (c *Context) Blob(code int, contentType string, b []byte) error {
	c.writeContentType(contentType)
	c.response.WriteHeader(code)
	if _, err := c.response.Write(b); err != nil {
		return err
	}

	return nil
}

// String writes s as a text/plain response.
func (c *Context) String(code int, s string) error {
	return c.Blob(code, textPlainUtf8, stringToBytes(s))
}

// HTML writes html as a text/html response.
func (c *Context) HTML(code int, html string) error {
	return c.Blob(code, textHtmlUtf8, stringToBytes(html))
}

// HTMLBlob writes blob as a text/html response.
func (c *Context) HTMLBlob(code int, blob []byte) error {
	return c.Blob(code, textHtmlUtf8, blob)
}

// JSON encodes j and writes it as an application/json response. j is
// fully encoded into memory before anything is written, so a write
// failure never leaves a partially-written body.
func (c *Context) JSON(code int, j any) error {
	c.writeContentType(applicationJson)

	buff := &bytes.Buffer{}
	enc := json.NewEncoder(buff)

	if err := enc.Encode(j); err != nil {
		return err
	}

	c.Response().WriteHeader(code)
	if _, err := buff.WriteTo(c.Response()); err != nil {
		return err
	}

	return nil
}

// Render renders the named view (name is the "controller/action" form
// used for files under ViewFS) and writes it as an HTML response. It also
// records name as the current action, which RenderBlock depends on to
// know which template set to render a block from — call Render before
// any RenderBlock call in the same request.
func (c *Context) Render(code int, name string, data any) error {
	c.action = name

	buf := &bytes.Buffer{}
	if err := c.trail.renderer.Render(c, buf, name, data); err != nil {
		return fmt.Errorf("context: render template %s: %w", name, err)
	}

	return c.HTMLBlob(code, buf.Bytes())
}

// RenderBlock renders the named block ({{define "block"}}...{{end}}) from
// the current action's template set — the one most recently passed to
// Render — without writing it to the response. It is the primitive
// cache.FetchFragment builds on to cache a portion of a page.
func (c *Context) RenderBlock(block string, data any) (template.HTML, error) {
	if c.action == "" {
		return "", fmt.Errorf("context: RenderBlock %q: no action rendered yet", block)
	}

	html, err := c.trail.renderer.RenderNamed(c.action, block, data)
	if err != nil {
		return "", fmt.Errorf("context: render block %s: %w", block, err)
	}

	return html, nil
}
