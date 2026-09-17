package cache_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/asimmons91/trails/cache"
	"github.com/asimmons91/trails/cache/backend/memory"
	"github.com/stretchr/testify/require"
)

func TestFetchCallsGeneratorOnMiss(t *testing.T) {
	store := memory.New()
	ctx := context.Background()

	calls := 0
	value, err := cache.Fetch(ctx, store, "greeting", time.Minute, func() (string, error) {
		calls++
		return "hello", nil
	})
	require.NoError(t, err)
	require.Equal(t, "hello", value)
	require.Equal(t, 1, calls)
}

func TestFetchDoesNotCallGeneratorOnHit(t *testing.T) {
	store := memory.New()
	ctx := context.Background()

	calls := 0
	generate := func() (string, error) {
		calls++
		return "hello", nil
	}

	_, err := cache.Fetch(ctx, store, "greeting", time.Minute, generate)
	require.NoError(t, err)

	value, err := cache.Fetch(ctx, store, "greeting", time.Minute, generate)
	require.NoError(t, err)
	require.Equal(t, "hello", value)
	require.Equal(t, 1, calls)
}

func TestFetchPropagatesGeneratorError(t *testing.T) {
	store := memory.New()
	ctx := context.Background()

	wantErr := errors.New("boom")
	_, err := cache.Fetch(ctx, store, "greeting", time.Minute, func() (string, error) {
		return "", wantErr
	})
	require.ErrorIs(t, err, wantErr)

	ok, err := store.Exist(ctx, "greeting")
	require.NoError(t, err)
	require.False(t, ok, "a failed generator must not be cached")
}

func TestWriteThenReadRoundTripsStruct(t *testing.T) {
	type user struct {
		Name string
		Age  int
	}

	store := memory.New()
	ctx := context.Background()

	want := user{Name: "Ada", Age: 30}
	require.NoError(t, cache.Write(ctx, store, "user:1", want, time.Minute))

	got, ok, err := cache.Read[user](ctx, store, "user:1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, want, got)
}

func TestReadMissReturnsNotOk(t *testing.T) {
	store := memory.New()
	ctx := context.Background()

	_, ok, err := cache.Read[string](ctx, store, "missing")
	require.NoError(t, err)
	require.False(t, ok)
}
