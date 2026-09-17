package cachetest_test

import (
	"context"
	"testing"
	"time"

	"github.com/asimmons91/trails/cache"
	"github.com/asimmons91/trails/cache/cachetest"
	"github.com/stretchr/testify/require"
)

func TestCountingStoreCountsReadsAndWrites(t *testing.T) {
	store := cachetest.New(nil)
	ctx := context.Background()

	_, _, _ = store.Read(ctx, "greeting")
	require.NoError(t, store.Write(ctx, "greeting", []byte("hi"), time.Minute))
	_, _, _ = store.Read(ctx, "greeting")

	require.Equal(t, 2, store.Reads("greeting"))
	require.Equal(t, 1, store.Writes("greeting"))
}

func TestFetchOnlyCallsGeneratorOnceThroughCountingStore(t *testing.T) {
	store := cachetest.New(nil)
	ctx := context.Background()

	calls := 0
	generate := func() (string, error) {
		calls++
		return "hello", nil
	}

	_, err := cache.Fetch(ctx, store, "greeting", time.Minute, generate)
	require.NoError(t, err)
	_, err = cache.Fetch(ctx, store, "greeting", time.Minute, generate)
	require.NoError(t, err)

	require.Equal(t, 1, calls)
	cachetest.AssertCached(t, ctx, store, "greeting")
	cachetest.AssertMiss(t, ctx, store, "missing")
}
