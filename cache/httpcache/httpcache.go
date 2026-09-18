// Package httpcache provides HTTP response-caching middleware built on a
// cache.Store: GET/HEAD requests that get a 200 OK response are cached
// (keyed, by default, on method+path+query) and served on a hit with
// ETag/If-None-Match support — a matching If-None-Match gets a bodyless
// 304. Any other method, and any non-200 response, is passed straight
// through, neither served from nor written to the cache.
//
//	t.Use(httpcache.Middleware(store))
package httpcache

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"net/http"
	"time"

	trails "github.com/asimmons91/trails"
	"github.com/asimmons91/trails/cache"
)

const defaultTTL = 5 * time.Minute

// KeyFunc derives a cache key from the incoming request. The default
// (method + path + raw query string) gives GET and HEAD requests to the
// same URL distinct entries and varies by every query parameter; use
// WithKeyFunc to share entries across requests that don't affect the
// response.
type KeyFunc func(r *http.Request) string

func defaultKeyFunc(r *http.Request) string {
	return r.Method + " " + r.URL.Path + "?" + r.URL.RawQuery
}

type config struct {
	ttl     time.Duration
	keyFunc KeyFunc
}

// Option configures Middleware.
type Option func(*config)

// WithTTL sets how long a cached response is served before it's treated
// as a miss. The default is 5 minutes.
func WithTTL(d time.Duration) Option {
	return func(c *config) { c.ttl = d }
}

// WithKeyFunc overrides the default KeyFunc used to derive a cache key
// from each request.
func WithKeyFunc(fn KeyFunc) Option {
	return func(c *config) { c.keyFunc = fn }
}

type cachedResponse struct {
	Status int
	Header http.Header
	Body   []byte
	ETag   string
}

// Middleware caches GET/HEAD responses in store. Only a 200 OK response is
// cached; anything else is written straight through uncached. A cache hit
// is served with the response's original headers and status plus an
// ETag; if the request's If-None-Match matches that ETag, a bodyless 304
// is served instead of the cached body.
func Middleware(store cache.Store, opts ...Option) trails.MiddlewareFunc {
	cfg := &config{ttl: defaultTTL, keyFunc: defaultKeyFunc}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(next trails.HandlerFunc) trails.HandlerFunc {
		return func(c *trails.Context) error {
			r := c.Request()
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				return next(c)
			}

			ctx := r.Context()
			key := cfg.keyFunc(r)

			if cached, ok := lookup(ctx, store, key); ok {
				return serveCached(c, r, cached)
			}

			return recordAndStore(c, next, store, ctx, key, cfg.ttl)
		}
	}
}

func lookup(ctx context.Context, store cache.Store, key string) (cachedResponse, bool) {
	raw, ok, err := store.Read(ctx, key)
	if err != nil || !ok {
		return cachedResponse{}, false
	}

	var cached cachedResponse
	if err := gob.NewDecoder(bytes.NewReader(raw)).Decode(&cached); err != nil {
		return cachedResponse{}, false
	}

	return cached, true
}

func serveCached(c *trails.Context, r *http.Request, cached cachedResponse) error {
	w := c.Response()

	if match := r.Header.Get("If-None-Match"); match != "" && match == cached.ETag {
		w.Header().Set("ETag", cached.ETag)
		w.WriteHeader(http.StatusNotModified)
		return nil
	}

	copyHeader(w.Header(), cached.Header)
	w.Header().Set("ETag", cached.ETag)
	w.WriteHeader(cached.Status)
	_, err := w.Write(cached.Body)
	return err
}

func recordAndStore(c *trails.Context, next trails.HandlerFunc, store cache.Store, ctx context.Context, key string, ttl time.Duration) error {
	original := c.Response()
	rec := newRecorder()
	c.SetResponse(rec)

	err := next(c)
	c.SetResponse(original)

	if err != nil {
		return err
	}

	status := rec.status
	if status == 0 {
		status = http.StatusOK
	}

	if status != http.StatusOK {
		copyHeader(original.Header(), rec.header)
		original.WriteHeader(status)
		_, werr := original.Write(rec.body.Bytes())
		return werr
	}

	etag := computeETag(rec.body.Bytes())
	cached := cachedResponse{Status: status, Header: rec.header, Body: rec.body.Bytes(), ETag: etag}

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(cached); err == nil {
		_ = store.Write(ctx, key, buf.Bytes(), ttl)
	}

	copyHeader(original.Header(), rec.header)
	original.Header().Set("ETag", etag)
	original.WriteHeader(status)
	_, werr := original.Write(rec.body.Bytes())
	return werr
}

func computeETag(body []byte) string {
	sum := sha256.Sum256(body)
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}

func copyHeader(dst, src http.Header) {
	for k, vals := range src {
		for _, v := range vals {
			dst.Add(k, v)
		}
	}
}

// recorder buffers a handler's response so it can be cached before being
// flushed to the real ResponseWriter.
type recorder struct {
	header      http.Header
	status      int
	body        bytes.Buffer
	wroteHeader bool
}

func newRecorder() *recorder {
	return &recorder{header: make(http.Header)}
}

func (r *recorder) Header() http.Header { return r.header }

func (r *recorder) WriteHeader(code int) {
	if r.wroteHeader {
		return
	}
	r.status = code
	r.wroteHeader = true
}

func (r *recorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.body.Write(b)
}
