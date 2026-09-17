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

type CountingStore struct {
	store cache.Store

	mu     sync.Mutex
	reads  map[string]int
	writes map[string]int
}

var _ cache.Store = (*CountingStore)(nil)

func New(store cache.Store) *CountingStore {
	if store == nil {
		store = memory.New()
	}
	return &CountingStore{
		store:  store,
		reads:  make(map[string]int),
		writes: make(map[string]int),
	}
}

func (c *CountingStore) Read(ctx context.Context, key string) ([]byte, bool, error) {
	c.mu.Lock()
	c.reads[key]++
	c.mu.Unlock()
	return c.store.Read(ctx, key)
}

func (c *CountingStore) Write(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	c.mu.Lock()
	c.writes[key]++
	c.mu.Unlock()
	return c.store.Write(ctx, key, value, ttl)
}

func (c *CountingStore) Delete(ctx context.Context, key string) error {
	return c.store.Delete(ctx, key)
}

func (c *CountingStore) Exist(ctx context.Context, key string) (bool, error) {
	return c.store.Exist(ctx, key)
}

func (c *CountingStore) Clear(ctx context.Context) error {
	return c.store.Clear(ctx)
}

// Reads returns how many times key has been read.
func (c *CountingStore) Reads(key string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reads[key]
}

func (c *CountingStore) Writes(key string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writes[key]
}

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
