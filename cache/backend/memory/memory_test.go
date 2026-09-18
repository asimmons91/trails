package memory_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/asimmons91/trails/cache/backend/memory"
	"github.com/stretchr/testify/require"
)

func TestWriteThenReadReturnsValue(t *testing.T) {
	b := memory.New()
	ctx := context.Background()

	require.NoError(t, b.Write(ctx, "greeting", []byte("hello"), time.Minute))

	value, ok, err := b.Read(ctx, "greeting")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "hello", string(value))
}

func TestReadMissingKeyReturnsNotOk(t *testing.T) {
	b := memory.New()
	ctx := context.Background()

	value, ok, err := b.Read(ctx, "missing")
	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, value)
}

func TestReadExpiredEntryReturnsNotOk(t *testing.T) {
	b := memory.New()
	ctx := context.Background()

	require.NoError(t, b.Write(ctx, "greeting", []byte("hello"), time.Nanosecond))
	time.Sleep(time.Millisecond)

	value, ok, err := b.Read(ctx, "greeting")
	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, value)
}

func TestWriteWithZeroTTLNeverExpires(t *testing.T) {
	b := memory.New()
	ctx := context.Background()

	require.NoError(t, b.Write(ctx, "greeting", []byte("hello"), 0))
	time.Sleep(time.Millisecond)

	_, ok, err := b.Read(ctx, "greeting")
	require.NoError(t, err)
	require.True(t, ok)
}

func TestExistReflectsExpiry(t *testing.T) {
	b := memory.New()
	ctx := context.Background()

	require.NoError(t, b.Write(ctx, "greeting", []byte("hello"), time.Nanosecond))
	time.Sleep(time.Millisecond)

	ok, err := b.Exist(ctx, "greeting")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestDeleteRemovesEntry(t *testing.T) {
	b := memory.New()
	ctx := context.Background()

	require.NoError(t, b.Write(ctx, "greeting", []byte("hello"), time.Minute))
	require.NoError(t, b.Delete(ctx, "greeting"))

	_, ok, err := b.Read(ctx, "greeting")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestClearRemovesAllEntries(t *testing.T) {
	b := memory.New()
	ctx := context.Background()

	require.NoError(t, b.Write(ctx, "a", []byte("1"), time.Minute))
	require.NoError(t, b.Write(ctx, "b", []byte("2"), time.Minute))
	require.NoError(t, b.Clear(ctx))

	_, ok, err := b.Read(ctx, "a")
	require.NoError(t, err)
	require.False(t, ok)

	_, ok, err = b.Read(ctx, "b")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestReadReturnsCopyNotSharedSlice(t *testing.T) {
	b := memory.New()
	ctx := context.Background()

	original := []byte("hello")
	require.NoError(t, b.Write(ctx, "greeting", original, time.Minute))
	original[0] = 'H'

	value, ok, err := b.Read(ctx, "greeting")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "hello", string(value))
}

func TestIncrementFreshKeyReturnsDelta(t *testing.T) {
	b := memory.New()
	ctx := context.Background()

	count, expiresAt, err := b.Increment(ctx, "counter", 1, time.Minute)
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
	require.WithinDuration(t, time.Now().Add(time.Minute), expiresAt, time.Second)
}

func TestIncrementExistingKeyAddsDeltaAndKeepsOriginalExpiry(t *testing.T) {
	b := memory.New()
	ctx := context.Background()

	_, first, err := b.Increment(ctx, "counter", 1, time.Hour)
	require.NoError(t, err)

	count, second, err := b.Increment(ctx, "counter", 1, time.Nanosecond)
	require.NoError(t, err)
	require.Equal(t, int64(2), count)
	require.Equal(t, first, second)
}

func TestIncrementExpiredKeyResetsInsteadOfAccumulating(t *testing.T) {
	b := memory.New()
	ctx := context.Background()

	_, _, err := b.Increment(ctx, "counter", 1, time.Nanosecond)
	require.NoError(t, err)
	time.Sleep(time.Millisecond)

	count, _, err := b.Increment(ctx, "counter", 1, time.Minute)
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
}

func TestIncrementZeroTTLNeverExpires(t *testing.T) {
	b := memory.New()
	ctx := context.Background()

	_, expiresAt, err := b.Increment(ctx, "counter", 1, 0)
	require.NoError(t, err)
	require.True(t, expiresAt.IsZero())
}

func TestIncrementIsConcurrencySafe(t *testing.T) {
	b := memory.New()
	ctx := context.Background()

	const n = 100
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := b.Increment(ctx, "counter", 1, time.Minute)
			require.NoError(t, err)
		}()
	}
	wg.Wait()

	value, ok, err := b.Read(ctx, "counter")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "100", string(value))
}

func TestRunSweepsExpiredEntries(t *testing.T) {
	b := memory.New(memory.WithCleanupInterval(10 * time.Millisecond))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = b.Run(ctx) }()

	require.NoError(t, b.Write(ctx, "greeting", []byte("hello"), time.Nanosecond))

	require.Eventually(t, func() bool {
		ok, err := b.Exist(ctx, "greeting")
		return err == nil && !ok
	}, time.Second, 5*time.Millisecond)
}

func TestRunReturnsWhenContextCancelled(t *testing.T) {
	b := memory.New()
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- b.Run(ctx) }()

	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}
