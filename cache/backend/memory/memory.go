package memory

import (
	"context"
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

type Backend struct {
	mu              sync.Mutex
	entries         map[string]entry
	cleanupInterval time.Duration
}

type Option func(*Backend)

func WithCleanupInterval(d time.Duration) Option {
	return func(b *Backend) { b.cleanupInterval = d }
}

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

func (b *Backend) Delete(_ context.Context, key string) error {
	b.mu.Lock()
	delete(b.entries, key)
	b.mu.Unlock()
	return nil
}

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

func (b *Backend) Clear(_ context.Context) error {
	b.mu.Lock()
	b.entries = make(map[string]entry)
	b.mu.Unlock()
	return nil
}

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
