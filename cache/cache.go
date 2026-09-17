package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

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
