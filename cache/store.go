// Package cache provides a generic caching layer over a pluggable Store:
// Fetch, Read, and Write cache arbitrary JSON-encodable values, and
// FetchFragment caches rendered view fragments. Store itself only deals in
// raw bytes, keys, and TTLs — cache/backend/memory and
// cache/backend/database are the two Store implementations trails ships,
// cache/httpcache builds an HTTP response-caching middleware on a Store,
// and cache/cachetest provides a call-counting Store decorator plus
// assertion helpers for testing code that depends on one.
package cache

import (
	"context"
	"time"
)

// Store is the raw byte-oriented cache backend that Fetch, Read, Write,
// and FetchFragment build on. Values and keys are opaque to Store itself
// — encoding (e.g. Fetch/Read/Write's JSON envelope) is the caller's
// responsibility.
type Store interface {
	Read(ctx context.Context, key string) ([]byte, bool, error)
	Write(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Exist(ctx context.Context, key string) (bool, error)
	Clear(ctx context.Context) error

	// Increment atomically adds delta to the integer counter stored at
	// key and returns its new value and absolute expiry (zero
	// time.Time means it never expires). If key is absent or already
	// expired, it is (re)created with value delta and ttl applied (ttl
	// <= 0 means never expires, matching Write). If key exists and has
	// not expired, delta is added to its current value and its existing
	// expiry is left untouched — ttl is ignored in that case. This
	// mirrors ActiveSupport::Cache::MemoryStore's increment, which
	// reuses the original entry's expiration and only applies a new TTL
	// when creating a fresh entry, giving fixed-window counting rather
	// than a sliding window that never settles under continuous
	// traffic.
	//
	// The stored representation is decimal ASCII text, not the JSON
	// envelope cache.Read/cache.Write use — do not mix Increment with
	// Read/Write/Fetch on the same key.
	Increment(ctx context.Context, key string, delta int64, ttl time.Duration) (count int64, expiresAt time.Time, err error)
}
