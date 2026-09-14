package trails

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

type Context struct {
	trail    *Trail
	store    map[string]any
	lock     sync.RWMutex
	request  *http.Request
	response http.ResponseWriter
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

func (c *Context) Reset(w http.ResponseWriter, r *http.Request) {
	c.request = r
	c.response = w
	clear(c.store)
}

func (c *Context) Get[T any](key string) T {
	c.lock.RLock()
	v := c.store[key]
	c.lock.RUnlock()

	t, _ := v.(T)
	return t
}

func (c *Context) Set(key string, val any) {
	c.lock.Lock()
	if c.store == nil {
		c.store = make(map[string]any)
	}
	c.store[key] = val
	c.lock.Unlock()
}

func (c *Context) Request() *http.Request { return c.request }

func (c *Context) Response() http.ResponseWriter { return c.response }

func (c *Context) Logger() *slog.Logger { return c.trail.logger }

func (c *Context) Context() context.Context { return c.trail.context }

func (c *Context) Bind(target any) error {
	return c.trail.binder.Bind(c, target)
}

func (c *Context) Blob(code int, contentType string, b []byte) error {
	c.writeContentType(contentType)
	c.response.WriteHeader(code)
	if _, err := c.response.Write(b); err != nil {
		return err
	}

	return nil
}

func (c *Context) String(code int, s string) error {
	return c.Blob(code, textPlainUtf8, stringToBytes(s))
}

func (c *Context) HTML(code int, html string) error {
	return c.Blob(code, textHtmlUtf8, stringToBytes(html))
}

func (c *Context) HTMLBlob(code int, blob []byte) error {
	return c.Blob(code, textHtmlUtf8, blob)
}

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

func (c *Context) Render(code int, name string, data any) error {
	buf := &bytes.Buffer{}
	if err := c.trail.renderer.Render(c, buf, name, data); err != nil {
		return fmt.Errorf("context: render template %s: %w", name, err)
	}

	return c.HTMLBlob(code, buf.Bytes())
}
