package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Fetch returns the cached value at key, decoded into T, or calls fn to
// generate it on a miss (including a decode failure on a corrupt or
// incompatible cached entry, which is discarded and regenerated rather
// than returned as an error) and caches the result via Write before
// returning it. fn is only called on a miss; its error is returned as-is
// and nothing is cached.
func Fetch[T any](ctx context.Context, store Store, key string, ttl time.Duration, fn func() (T, error)) (T, error) {
	var zero T

	if raw, ok, err := store.Read(ctx, key); err != nil {
		return zero, fmt.Errorf("cache: reading %q: %w", key, err)
	} else if ok {
		var v T
		if err := json.Unmarshal(raw, &v); err == nil {
			return v, nil
		}
		// Corrupt or incompatible entry: fall through and regenerate.
	}

	v, err := fn()
	if err != nil {
		return zero, err
	}

	if err := Write(ctx, store, key, v, ttl); err != nil {
		return zero, err
	}

	return v, nil
}

// Read returns the cached value at key decoded into T. ok is false on a
// miss; a stored value that fails to decode into T is an error, unlike
// Fetch, which treats the same failure as a miss and regenerates.
func Read[T any](ctx context.Context, store Store, key string) (T, bool, error) {
	var zero T

	raw, ok, err := store.Read(ctx, key)
	if err != nil {
		return zero, false, fmt.Errorf("cache: reading %q: %w", key, err)
	}
	if !ok {
		return zero, false, nil
	}

	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return zero, false, fmt.Errorf("cache: decoding %q: %w", key, err)
	}

	return v, true, nil
}

// Write JSON-encodes val and stores it at key with the given ttl (ttl <=
// 0 means never expires, per Store).
func Write[T any](ctx context.Context, store Store, key string, val T, ttl time.Duration) error {
	raw, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("cache: encoding %q: %w", key, err)
	}

	if err := store.Write(ctx, key, raw, ttl); err != nil {
		return fmt.Errorf("cache: writing %q: %w", key, err)
	}

	return nil
}
