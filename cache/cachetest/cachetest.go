// Package cachetest provides test doubles for code that depends on a
// cache.Store: CountingStore wraps one, counting calls to Read, Write, and
// Increment (Delete/Exist/Clear pass through uncounted), and
// AssertCached/AssertMiss assert whether a key is currently cached.
package cachetest

import (
	"context"
	"sync"
	"time"

	"github.com/asimmons91/trails/cache"
	"github.com/asimmons91/trails/cache/backend/memory"
	"github.com/stretchr/testify/assert"
)

type tHelper interface {
	Helper()
}

// CountingStore wraps a cache.Store, counting calls to Read, Write, and
// Increment per key so a test can assert how many times each happened.
// Delete, Exist, and Clear are passed through to the wrapped store without
// being counted.
type CountingStore struct {
	store cache.Store

	mu         sync.Mutex
	reads      map[string]int
	writes     map[string]int
	increments map[string]int
}

var _ cache.Store = (*CountingStore)(nil)

// New wraps store, or a fresh cache/backend/memory Store if store is nil.
func New(store cache.Store) *CountingStore {
	if store == nil {
		store = memory.New()
	}
	return &CountingStore{
		store:      store,
		reads:      make(map[string]int),
		writes:     make(map[string]int),
		increments: make(map[string]int),
	}
}

// Read increments key's read count (see Reads) and delegates to the
// wrapped store.
func (c *CountingStore) Read(ctx context.Context, key string) ([]byte, bool, error) {
	c.mu.Lock()
	c.reads[key]++
	c.mu.Unlock()
	return c.store.Read(ctx, key)
}

// Write increments key's write count (see Writes) and delegates to the
// wrapped store.
func (c *CountingStore) Write(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	c.mu.Lock()
	c.writes[key]++
	c.mu.Unlock()
	return c.store.Write(ctx, key, value, ttl)
}

// Increment increments key's increment count (see Increments) and
// delegates to the wrapped store.
func (c *CountingStore) Increment(ctx context.Context, key string, delta int64, ttl time.Duration) (int64, time.Time, error) {
	c.mu.Lock()
	c.increments[key]++
	c.mu.Unlock()
	return c.store.Increment(ctx, key, delta, ttl)
}

// Delete delegates to the wrapped store, uncounted.
func (c *CountingStore) Delete(ctx context.Context, key string) error {
	return c.store.Delete(ctx, key)
}

// Exist delegates to the wrapped store, uncounted.
func (c *CountingStore) Exist(ctx context.Context, key string) (bool, error) {
	return c.store.Exist(ctx, key)
}

// Clear delegates to the wrapped store, uncounted.
func (c *CountingStore) Clear(ctx context.Context) error {
	return c.store.Clear(ctx)
}

// Reads returns how many times key has been read.
func (c *CountingStore) Reads(key string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reads[key]
}

// Writes returns how many times key has been written.
func (c *CountingStore) Writes(key string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writes[key]
}

// Increments returns how many times key has been incremented.
func (c *CountingStore) Increments(key string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.increments[key]
}

// AssertCached asserts that key currently exists in store (via
// store.Exist), failing t and returning false if it does not.
func AssertCached(t assert.TestingT, ctx context.Context, store cache.Store, key string) bool {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}

	ok, err := store.Exist(ctx, key)
	if !assert.NoError(t, err) {
		return false
	}
	return assert.True(t, ok, "expected %q to be cached, but it was not", key)
}

// AssertMiss asserts that key does not currently exist in store (via
// store.Exist), failing t and returning false if it does.
func AssertMiss(t assert.TestingT, ctx context.Context, store cache.Store, key string) bool {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}

	ok, err := store.Exist(ctx, key)
	if !assert.NoError(t, err) {
		return false
	}
	return assert.False(t, ok, "expected %q to be a cache miss, but it was cached", key)
}
