package mysql_test

import (
	"context"
	"testing"
	"time"

	"github.com/asimmons91/trails/channels"
	"github.com/asimmons91/trails/channels/backend/database"
	"github.com/asimmons91/trails/driver/mysql"
	"github.com/asimmons91/trails/pack"
	"github.com/asimmons91/trails/pack/migrate"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
)

func newChannelsDatabaseTestDB(t *testing.T) *pack.DB {
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

func TestDatabaseChannels_CrossInstanceDelivery(t *testing.T) {
	db := newChannelsDatabaseTestDB(t)
	ctx := context.Background()

	a, err := database.New(ctx, db, database.WithPollInterval(20*time.Millisecond))
	require.NoError(t, err)
	bb, err := database.New(ctx, db, database.WithPollInterval(20*time.Millisecond))
	require.NoError(t, err)

	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = a.Run(runCtx) }()
	go func() { _ = bb.Run(runCtx) }()

	subOnA, err := a.Subscribe(ctx, "room1")
	require.NoError(t, err)
	subOnB, err := bb.Subscribe(ctx, "room1")
	require.NoError(t, err)

	// Delivery is pure polling (no same-process fast path), so every
	// subscriber on the topic sees every message regardless of which
	// instance published it or which instance the subscriber is attached
	// to — including a subscriber seeing its own instance's publish.
	require.NoError(t, bb.Publish(ctx, "room1", []byte("from-b")))
	requireDatabaseTestReceives(t, subOnA, "from-b") // cross-instance: A observes B's write
	requireDatabaseTestReceives(t, subOnB, "from-b") // same-instance: B observes its own write

	require.NoError(t, a.Publish(ctx, "room1", []byte("from-a")))
	requireDatabaseTestReceives(t, subOnB, "from-a") // cross-instance: B observes A's write
	requireDatabaseTestReceives(t, subOnA, "from-a") // same-instance: A observes its own write
}

func requireDatabaseTestReceives(t *testing.T, sub channels.Subscription, want string) {
	t.Helper()
	select {
	case msg := <-sub.Messages():
		require.Equal(t, want, string(msg))
	case <-time.After(3 * time.Second):
		t.Fatalf("expected message %q, got none", want)
	}
}

func TestDatabaseChannels_TrimSweepsRowsPastRetention(t *testing.T) {
	db := newChannelsDatabaseTestDB(t)
	ctx := context.Background()

	b, err := database.New(ctx, db,
		database.WithPollInterval(10*time.Millisecond),
		database.WithRetention(50*time.Millisecond),
		database.WithTrimInterval(20*time.Millisecond),
	)
	require.NoError(t, err)

	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = b.Run(runCtx) }()

	require.NoError(t, b.Publish(ctx, "room1", []byte("hi")))

	type messageRowView struct {
		pack.Model[int64] `db:"table:channel_messages"`
	}
	require.Eventually(t, func() bool {
		n, err := pack.Of[messageRowView](db).Count(ctx)
		require.NoError(t, err)
		return n == 0
	}, 5*time.Second, 20*time.Millisecond, "row should be swept once past retention")
}
