package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/asimmons91/trails/channels"
	"github.com/asimmons91/trails/channels/backend/memory"
	"github.com/stretchr/testify/require"
)

func TestPublishDeliversToSubscriber(t *testing.T) {
	b := memory.New()
	defer b.Close()

	sub, err := b.Subscribe(context.Background(), "room1")
	require.NoError(t, err)

	require.NoError(t, b.Publish(context.Background(), "room1", []byte("hello")))
	requireReceives(t, sub, "hello")
}

func TestPublishToTopicWithNoSubscribersIsNoop(t *testing.T) {
	b := memory.New()
	defer b.Close()

	require.NoError(t, b.Publish(context.Background(), "empty", []byte("hi")))
}

func TestPublishFansOutToMultipleSubscribers(t *testing.T) {
	b := memory.New()
	defer b.Close()

	sub1, err := b.Subscribe(context.Background(), "room1")
	require.NoError(t, err)
	sub2, err := b.Subscribe(context.Background(), "room1")
	require.NoError(t, err)

	require.NoError(t, b.Publish(context.Background(), "room1", []byte("hi")))

	requireReceives(t, sub1, "hi")
	requireReceives(t, sub2, "hi")
}

func TestPublishOnlyReachesSubscribersOfThatTopic(t *testing.T) {
	b := memory.New()
	defer b.Close()

	room1, err := b.Subscribe(context.Background(), "room1")
	require.NoError(t, err)
	room2, err := b.Subscribe(context.Background(), "room2")
	require.NoError(t, err)

	require.NoError(t, b.Publish(context.Background(), "room1", []byte("hi")))

	requireReceives(t, room1, "hi")
	requireNoMessage(t, room2)
}

func TestUnsubscribeStopsDeliveryAndClosesChannel(t *testing.T) {
	b := memory.New()
	defer b.Close()

	sub, err := b.Subscribe(context.Background(), "room1")
	require.NoError(t, err)
	require.NoError(t, sub.Unsubscribe())

	require.NoError(t, b.Publish(context.Background(), "room1", []byte("late")))

	_, ok := <-sub.Messages()
	require.False(t, ok, "messages channel should be closed after Unsubscribe")
}

func TestUnsubscribeIsIdempotent(t *testing.T) {
	b := memory.New()
	defer b.Close()

	sub, err := b.Subscribe(context.Background(), "room1")
	require.NoError(t, err)

	require.NoError(t, sub.Unsubscribe())
	require.NoError(t, sub.Unsubscribe()) // must not panic on double-close
}

func TestCloseUnblocksAllSubscriptions(t *testing.T) {
	b := memory.New()

	sub, err := b.Subscribe(context.Background(), "room1")
	require.NoError(t, err)

	require.NoError(t, b.Close())

	_, ok := <-sub.Messages()
	require.False(t, ok)
}

func TestCloseIsIdempotent(t *testing.T) {
	b := memory.New()
	require.NoError(t, b.Close())
	require.NoError(t, b.Close())
}

func TestSubscribeAfterCloseReturnsErrClosed(t *testing.T) {
	b := memory.New()
	require.NoError(t, b.Close())

	_, err := b.Subscribe(context.Background(), "room1")
	require.ErrorIs(t, err, memory.ErrClosed)
}

func TestPublishAfterCloseReturnsErrClosed(t *testing.T) {
	b := memory.New()
	require.NoError(t, b.Close())

	err := b.Publish(context.Background(), "room1", []byte("hi"))
	require.ErrorIs(t, err, memory.ErrClosed)
}

func requireReceives(t *testing.T, sub channels.Subscription, want string) {
	t.Helper()
	select {
	case msg := <-sub.Messages():
		require.Equal(t, want, string(msg))
	case <-time.After(time.Second):
		t.Fatal("expected a message, got none")
	}
}

func requireNoMessage(t *testing.T, sub channels.Subscription) {
	t.Helper()
	select {
	case msg := <-sub.Messages():
		t.Fatalf("expected no message, got %q", msg)
	case <-time.After(100 * time.Millisecond):
	}
}
