package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/asimmons91/trails/channels"
	"github.com/asimmons91/trails/channels/backend/database"
	"github.com/asimmons91/trails/pack"
	"github.com/asimmons91/trails/pack/driver/sqlite"
	"github.com/asimmons91/trails/pack/migrate"
	"github.com/stretchr/testify/require"
)

type messageRowView struct {
	pack.Model[int64] `db:"table:channel_messages"`
	Topic             string `db:"topic"`
	Payload           string `db:"payload"`
	CreatedAt         int64  `db:"created_at"`
}

func newChannelsTestDB(t *testing.T) *pack.DB {
	t.Helper()

	d := sqlite.New()
	sqlDB, err := d.Open(":memory:")
	require.NoError(t, err)
	// Run polls and Publish write concurrently; a fresh connection per query
	// against ":memory:" would each see an independent empty database, so
	// pin the pool to the one connection that ran migrations.
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	db := pack.Open(sqlDB, d.Dialect())
	require.NoError(t, database.Migration.Migrate(context.Background(), migrate.New(db)))
	return db
}

func runChannelsBackend(t *testing.T, b *database.Backend) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- b.Run(ctx) }()

	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("database.Backend.Run did not stop after context cancellation")
		}
	})
}

func newTestBackend(t *testing.T, opts ...database.Option) (*pack.DB, *database.Backend) {
	t.Helper()

	db := newChannelsTestDB(t)
	opts = append([]database.Option{database.WithPollInterval(10 * time.Millisecond)}, opts...)
	b, err := database.New(context.Background(), db, opts...)
	require.NoError(t, err)
	runChannelsBackend(t, b)
	return db, b
}

func requireReceives(t *testing.T, sub channels.Subscription, want string) {
	t.Helper()
	select {
	case msg := <-sub.Messages():
		require.Equal(t, want, string(msg))
	case <-time.After(3 * time.Second):
		t.Fatal("expected a message, got none")
	}
}

func requireNoMessage(t *testing.T, sub channels.Subscription) {
	t.Helper()
	select {
	case msg := <-sub.Messages():
		t.Fatalf("expected no message, got %q", msg)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestDatabase_PublishDeliversToSubscriber(t *testing.T) {
	_, b := newTestBackend(t)

	sub, err := b.Subscribe(context.Background(), "room1")
	require.NoError(t, err)

	require.NoError(t, b.Publish(context.Background(), "room1", []byte("hello")))
	requireReceives(t, sub, "hello")
}

func TestDatabase_PublishToTopicWithNoSubscribersIsNoop(t *testing.T) {
	_, b := newTestBackend(t)

	require.NoError(t, b.Publish(context.Background(), "empty", []byte("hi")))
}

func TestDatabase_PublishFansOutToMultipleSubscribers(t *testing.T) {
	_, b := newTestBackend(t)

	sub1, err := b.Subscribe(context.Background(), "room1")
	require.NoError(t, err)
	sub2, err := b.Subscribe(context.Background(), "room1")
	require.NoError(t, err)

	require.NoError(t, b.Publish(context.Background(), "room1", []byte("hi")))

	requireReceives(t, sub1, "hi")
	requireReceives(t, sub2, "hi")
}

func TestDatabase_PublishOnlyReachesSubscribersOfThatTopic(t *testing.T) {
	_, b := newTestBackend(t)

	room1, err := b.Subscribe(context.Background(), "room1")
	require.NoError(t, err)
	room2, err := b.Subscribe(context.Background(), "room2")
	require.NoError(t, err)

	require.NoError(t, b.Publish(context.Background(), "room1", []byte("hi")))

	requireReceives(t, room1, "hi")
	requireNoMessage(t, room2)
}

func TestDatabase_UnsubscribeStopsDeliveryAndClosesChannel(t *testing.T) {
	_, b := newTestBackend(t)

	sub, err := b.Subscribe(context.Background(), "room1")
	require.NoError(t, err)
	require.NoError(t, sub.Unsubscribe())

	require.NoError(t, b.Publish(context.Background(), "room1", []byte("late")))

	time.Sleep(150 * time.Millisecond) // let the poll loop run at least once
	_, ok := <-sub.Messages()
	require.False(t, ok, "messages channel should be closed after Unsubscribe")
}

func TestDatabase_CloseUnblocksAllSubscriptions(t *testing.T) {
	_, b := newTestBackend(t)

	sub, err := b.Subscribe(context.Background(), "room1")
	require.NoError(t, err)

	require.NoError(t, b.Close())

	_, ok := <-sub.Messages()
	require.False(t, ok)
}

func TestDatabase_SubscribeAfterCloseReturnsErrClosed(t *testing.T) {
	_, b := newTestBackend(t)
	require.NoError(t, b.Close())

	_, err := b.Subscribe(context.Background(), "room1")
	require.ErrorIs(t, err, database.ErrClosed)
}

func TestDatabase_PublishAfterCloseReturnsErrClosed(t *testing.T) {
	_, b := newTestBackend(t)
	require.NoError(t, b.Close())

	err := b.Publish(context.Background(), "room1", []byte("hi"))
	require.ErrorIs(t, err, database.ErrClosed)
}

// TestDatabase_SubscriberNeverSeesMessagesPublishedBeforeBackendStarted
// confirms New's cursor initialization: messages already in the table when
// a Backend starts are not redelivered to it, matching ActionCable/
// SolidCable "live broadcast" semantics rather than a durable log.
func TestDatabase_SubscriberNeverSeesMessagesPublishedBeforeBackendStarted(t *testing.T) {
	db := newChannelsTestDB(t)

	require.NoError(t, pack.Create(context.Background(), db, &messageRowView{
		Topic:     "room1",
		Payload:   "old",
		CreatedAt: time.Now().UnixNano(),
	}))

	b, err := database.New(context.Background(), db, database.WithPollInterval(10*time.Millisecond))
	require.NoError(t, err)
	runChannelsBackend(t, b)

	sub, err := b.Subscribe(context.Background(), "room1")
	require.NoError(t, err)
	requireNoMessage(t, sub)

	require.NoError(t, b.Publish(context.Background(), "room1", []byte("new")))
	requireReceives(t, sub, "new")
}

func TestDatabase_PollDrainsBacklogWithinOneTick(t *testing.T) {
	_, b := newTestBackend(t, database.WithBatchSize(3), database.WithPollInterval(50*time.Millisecond))

	sub, err := b.Subscribe(context.Background(), "room1")
	require.NoError(t, err)

	const n = 10
	for i := range n {
		require.NoError(t, b.Publish(context.Background(), "room1", []byte{byte('a' + i)}))
	}

	for i := range n {
		requireReceives(t, sub, string(rune('a'+i)))
	}
}

func TestDatabase_TrimSweepsRowsPastRetention(t *testing.T) {
	db, b := newTestBackend(t,
		database.WithRetention(50*time.Millisecond),
		database.WithTrimInterval(20*time.Millisecond),
	)

	require.NoError(t, b.Publish(context.Background(), "room1", []byte("hi")))

	require.Eventually(t, func() bool {
		n, err := pack.Of[messageRowView](db).Count(context.Background())
		require.NoError(t, err)
		return n == 0
	}, 3*time.Second, 10*time.Millisecond, "row should be swept once past retention")
}
