package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/asimmons91/trails/cache"
	"github.com/asimmons91/trails/cache/backend/database"
	"github.com/asimmons91/trails/pack"
	"github.com/asimmons91/trails/pack/driver/sqlite"
	"github.com/asimmons91/trails/pack/migrate"
	"github.com/stretchr/testify/require"
)

func newCacheTestDB(t *testing.T) *pack.DB {
	t.Helper()

	d := sqlite.New()
	sqlDB, err := d.Open(":memory:")
	require.NoError(t, err)
	// A fresh connection per query against ":memory:" would each see an
	// independent empty database, so pin the pool to the one connection
	// that ran migrations.
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	db := pack.Open(sqlDB, d.Dialect())
	require.NoError(t, database.Migration.Migrate(context.Background(), migrate.New(db)))
	return db
}

func TestCacheDatabase_WriteThenReadReturnsValue(t *testing.T) {
	db := newCacheTestDB(t)
	b := database.New(db)
	ctx := context.Background()

	require.NoError(t, b.Write(ctx, "greeting", []byte("hello"), time.Minute))

	value, ok, err := b.Read(ctx, "greeting")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "hello", string(value))
}

func TestCacheDatabase_ReadMissingKeyReturnsNotOk(t *testing.T) {
	db := newCacheTestDB(t)
	b := database.New(db)
	ctx := context.Background()

	value, ok, err := b.Read(ctx, "missing")
	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, value)
}

func TestCacheDatabase_ReadExpiredEntryReturnsNotOk(t *testing.T) {
	db := newCacheTestDB(t)
	b := database.New(db)
	ctx := context.Background()

	require.NoError(t, b.Write(ctx, "greeting", []byte("hello"), time.Nanosecond))
	time.Sleep(time.Millisecond)

	value, ok, err := b.Read(ctx, "greeting")
	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, value)
}

func TestCacheDatabase_WriteWithZeroTTLNeverExpires(t *testing.T) {
	db := newCacheTestDB(t)
	b := database.New(db)
	ctx := context.Background()

	require.NoError(t, b.Write(ctx, "greeting", []byte("hello"), 0))
	time.Sleep(time.Millisecond)

	_, ok, err := b.Read(ctx, "greeting")
	require.NoError(t, err)
	require.True(t, ok)
}

func TestCacheDatabase_WriteTwiceUpsertsInPlace(t *testing.T) {
	db := newCacheTestDB(t)
	b := database.New(db)
	ctx := context.Background()

	require.NoError(t, b.Write(ctx, "greeting", []byte("hello"), time.Minute))
	require.NoError(t, b.Write(ctx, "greeting", []byte("goodbye"), time.Minute))

	value, ok, err := b.Read(ctx, "greeting")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "goodbye", string(value))

	n, err := pack.Of[entryRowView](db).Count(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), n, "upsert must not create a second row for the same key")
}

func TestCacheDatabase_ExistReflectsExpiry(t *testing.T) {
	db := newCacheTestDB(t)
	b := database.New(db)
	ctx := context.Background()

	require.NoError(t, b.Write(ctx, "greeting", []byte("hello"), time.Nanosecond))
	time.Sleep(time.Millisecond)

	ok, err := b.Exist(ctx, "greeting")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestCacheDatabase_DeleteRemovesEntry(t *testing.T) {
	db := newCacheTestDB(t)
	b := database.New(db)
	ctx := context.Background()

	require.NoError(t, b.Write(ctx, "greeting", []byte("hello"), time.Minute))
	require.NoError(t, b.Delete(ctx, "greeting"))

	_, ok, err := b.Read(ctx, "greeting")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestCacheDatabase_ClearRemovesAllEntries(t *testing.T) {
	db := newCacheTestDB(t)
	b := database.New(db)
	ctx := context.Background()

	require.NoError(t, b.Write(ctx, "a", []byte("1"), time.Minute))
	require.NoError(t, b.Write(ctx, "b", []byte("2"), time.Minute))
	require.NoError(t, b.Clear(ctx))

	n, err := pack.Of[entryRowView](db).Count(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(0), n)
}

func TestCacheDatabase_ImplementsCacheStore(t *testing.T) {
	var _ cache.Store = database.New(newCacheTestDB(t))
}

func TestCacheDatabase_RunSweepsExpiredEntriesInBatches(t *testing.T) {
	db := newCacheTestDB(t)
	b := database.New(db,
		database.WithSweepInterval(10*time.Millisecond),
		database.WithSweepBatchSize(2),
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- b.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("Run did not stop after context cancellation")
		}
	})

	// More rows than one sweep batch, all already expired.
	for i := range 5 {
		require.NoError(t, b.Write(ctx, string(rune('a'+i)), []byte("x"), time.Nanosecond))
	}
	require.NoError(t, b.Write(ctx, "keep", []byte("still here"), time.Minute))
	time.Sleep(2 * time.Millisecond)

	require.Eventually(t, func() bool {
		n, err := pack.Of[entryRowView](db).Count(context.Background())
		require.NoError(t, err)
		return n == 1
	}, 3*time.Second, 10*time.Millisecond, "all expired rows should be swept across multiple batches, leaving only the unexpired one")

	_, ok, err := b.Read(ctx, "keep")
	require.NoError(t, err)
	require.True(t, ok)
}

func TestCacheDatabase_MigrationRollbackDropsTable(t *testing.T) {
	db := newCacheTestDB(t)
	m := migrate.New(db)

	require.NoError(t, database.Migration.Rollback(context.Background(), m))

	hasTable, err := m.HasTable(context.Background(), &entryRowView{})
	require.NoError(t, err)
	require.False(t, hasTable)
}

// entryRowView mirrors cache/backend/database's unexported entryRow so
// tests in this package can query the table directly.
type entryRowView struct {
	pack.Model[int64] `db:"table:cache_entries"`
	Key               string `db:"key"`
	Value             []byte `db:"value"`
	ExpiresAt         int64  `db:"expires_at"`
	CreatedAt         int64  `db:"created_at"`
}
