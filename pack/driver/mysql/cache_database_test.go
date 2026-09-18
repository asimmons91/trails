package mysql_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/asimmons91/trails/cache/backend/database"
	"github.com/asimmons91/trails/pack"
	"github.com/asimmons91/trails/pack/driver/mysql"
	"github.com/asimmons91/trails/pack/migrate"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
)

func newCacheDatabaseTestDB(t *testing.T) *pack.DB {
	t.Helper()
	ctx := context.Background()

	mc, err := tcmysql.Run(ctx,
		"mysql:8",
		tcmysql.WithDatabase("pack"),
		tcmysql.WithUsername("pack"),
		tcmysql.WithPassword("pack"),
	)
	testcontainers.CleanupContainer(t, mc)
	require.NoError(t, err)

	connStr, err := mc.ConnectionString(ctx)
	require.NoError(t, err)

	db, err := pack.Connect(mysql.New(), connStr)
	require.NoError(t, err)

	require.NoError(t, database.Migration.Migrate(ctx, migrate.New(db)))
	return db
}

func TestCacheDatabase_WriteReadUpsertAndSweep(t *testing.T) {
	t.Parallel()
	db := newCacheDatabaseTestDB(t)
	ctx := context.Background()
	b := database.New(db, database.WithSweepInterval(20*time.Millisecond))

	require.NoError(t, b.Write(ctx, "greeting", []byte("hello"), time.Minute))
	value, ok, err := b.Read(ctx, "greeting")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "hello", string(value))

	// Upsert on the same key must overwrite in place (INSERT ... ON
	// DUPLICATE KEY UPDATE on MySQL), not error on the unique key.
	require.NoError(t, b.Write(ctx, "greeting", []byte("goodbye"), time.Minute))
	value, ok, err = b.Read(ctx, "greeting")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "goodbye", string(value))

	// An expired entry is swept by Run in the background.
	require.NoError(t, b.Write(ctx, "temp", []byte("x"), time.Nanosecond))
	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = b.Run(runCtx) }()

	require.Eventually(t, func() bool {
		ok, err := b.Exist(ctx, "temp")
		return err == nil && !ok
	}, 3*time.Second, 20*time.Millisecond, "expired entry should be swept")
}

func TestCacheDatabase_Increment(t *testing.T) {
	t.Parallel()
	db := newCacheDatabaseTestDB(t)
	ctx := context.Background()
	b := database.New(db)

	count, expiresAt, err := b.Increment(ctx, "counter", 1, time.Minute)
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
	require.WithinDuration(t, time.Now().Add(time.Minute), expiresAt, 5*time.Second)

	// A second increment within the window adds delta and keeps the
	// original expiry (ttl is ignored once the key already exists).
	count, second, err := b.Increment(ctx, "counter", 1, time.Nanosecond)
	require.NoError(t, err)
	require.Equal(t, int64(2), count)
	require.Equal(t, expiresAt, second)

	// An expired key resets instead of accumulating.
	require.NoError(t, b.Write(ctx, "expired", []byte("0"), time.Nanosecond))
	time.Sleep(time.Millisecond)
	count, _, err = b.Increment(ctx, "expired", 1, time.Minute)
	require.NoError(t, err)
	require.Equal(t, int64(1), count)

	// A zero TTL never expires.
	_, zeroExpiry, err := b.Increment(ctx, "forever", 1, 0)
	require.NoError(t, err)
	require.True(t, zeroExpiry.IsZero())

	// Concurrent increments on the same key must not lose updates.
	const n = 50
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := b.Increment(ctx, "concurrent", 1, time.Minute)
			require.NoError(t, err)
		}()
	}
	wg.Wait()

	value, ok, err := b.Read(ctx, "concurrent")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "50", string(value))
}
