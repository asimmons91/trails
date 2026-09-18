// Package memory provides an in-process, non-persistent cache.Store: all
// entries live in a map guarded by a mutex and are lost on process
// restart. Run periodically sweeps expired entries and satisfies
// trails.Runner; without it, an expired entry is only actually removed
// the next time it's read, checked, or incremented (Read/Exist/Increment
// all evict lazily on access), so memory held by keys nobody touches
// again isn't reclaimed until Run's sweep catches it.
package memory

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	trails "github.com/asimmons91/trails"
	"github.com/asimmons91/trails/cache"
)

const defaultCleanupInterval = time.Minute

var (
	_ cache.Store   = (*Backend)(nil)
	_ trails.Runner = (*Backend)(nil)
)

type entry struct {
	value     []byte
	expiresAt time.Time // zero means no expiry
}

func (e entry) expired(now time.Time) bool {
	return !e.expiresAt.IsZero() && now.After(e.expiresAt)
}

// Backend is an in-process, in-memory cache.Store. Construct one with New.
type Backend struct {
	mu              sync.Mutex
	entries         map[string]entry
	cleanupInterval time.Duration
}

// Option configures New.
type Option func(*Backend)

// WithCleanupInterval sets how often Run sweeps expired entries. The
// default is one minute.
func WithCleanupInterval(d time.Duration) Option {
	return func(b *Backend) { b.cleanupInterval = d }
}

// New returns a ready-to-use Backend.
func New(opts ...Option) *Backend {
	b := &Backend{
		entries:         make(map[string]entry),
		cleanupInterval: defaultCleanupInterval,
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// Read returns the value stored at key, and false if key is absent or has
// expired. An expired entry is evicted as a side effect of this call.
func (b *Backend) Read(_ context.Context, key string) ([]byte, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	e, ok := b.entries[key]
	if !ok {
		return nil, false, nil
	}
	if e.expired(time.Now()) {
		delete(b.entries, key)
		return nil, false, nil
	}

	value := make([]byte, len(e.value))
	copy(value, e.value)
	return value, true, nil
}

// Write stores value at key, replacing any existing entry, expiring after
// ttl (ttl <= 0 means never).
func (b *Backend) Write(_ context.Context, key string, value []byte, ttl time.Duration) error {
	stored := make([]byte, len(value))
	copy(stored, value)

	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	}

	b.mu.Lock()
	b.entries[key] = entry{value: stored, expiresAt: expiresAt}
	b.mu.Unlock()

	return nil
}

// Delete removes key. It is not an error if key doesn't exist.
func (b *Backend) Delete(_ context.Context, key string) error {
	b.mu.Lock()
	delete(b.entries, key)
	b.mu.Unlock()
	return nil
}

// Exist reports whether key is present and unexpired, evicting it as a
// side effect if it has expired.
func (b *Backend) Exist(_ context.Context, key string) (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	e, ok := b.entries[key]
	if !ok {
		return false, nil
	}
	if e.expired(time.Now()) {
		delete(b.entries, key)
		return false, nil
	}

	return true, nil
}

// Increment implements cache.Store's Increment contract (see Store's doc
// comment for the full semantics) in memory, under b's own mutex.
func (b *Backend) Increment(_ context.Context, key string, delta int64, ttl time.Duration) (int64, time.Time, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	e, ok := b.entries[key]

	if !ok || e.expired(now) {
		var expiresAt time.Time
		if ttl > 0 {
			expiresAt = now.Add(ttl)
		}
		b.entries[key] = entry{value: []byte(strconv.FormatInt(delta, 10)), expiresAt: expiresAt}
		return delta, expiresAt, nil
	}

	cur, err := strconv.ParseInt(string(e.value), 10, 64)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("memory: increment %q: stored value is not an integer counter: %w", key, err)
	}

	next := cur + delta
	b.entries[key] = entry{value: []byte(strconv.FormatInt(next, 10)), expiresAt: e.expiresAt}
	return next, e.expiresAt, nil
}

// Clear removes every entry.
func (b *Backend) Clear(_ context.Context) error {
	b.mu.Lock()
	b.entries = make(map[string]entry)
	b.mu.Unlock()
	return nil
}

// Run sweeps expired entries every cleanupInterval until ctx is
// cancelled. It satisfies trails.Runner.
func (b *Backend) Run(ctx context.Context) error {
	ticker := time.NewTicker(b.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			b.sweep()
		}
	}
}

func (b *Backend) sweep() {
	now := time.Now()

	b.mu.Lock()
	defer b.mu.Unlock()

	for key, e := range b.entries {
		if e.expired(now) {
			delete(b.entries, key)
		}
	}
}
