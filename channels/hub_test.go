package channels_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/asimmons91/trails/channels"
	"github.com/asimmons91/trails/channels/backend/memory"
	"github.com/stretchr/testify/require"
)

type stubChannel struct {
	subscribedCalls   []map[string]string
	unsubscribedCalls int
	receivedCalls     []json.RawMessage
	subscribeErr      error
	streamTopic       string // topic to StreamFrom during Subscribed, if set
}

func (c *stubChannel) Name() string { return "stub" }

func (c *stubChannel) Subscribed(ctx context.Context, sub *channels.Subscriber, params map[string]string) error {
	c.subscribedCalls = append(c.subscribedCalls, params)
	if c.subscribeErr != nil {
		return c.subscribeErr
	}
	if c.streamTopic != "" {
		return sub.StreamFrom(ctx, c.streamTopic)
	}
	return nil
}

func (c *stubChannel) Unsubscribed(ctx context.Context, sub *channels.Subscriber) {
	c.unsubscribedCalls++
}

func (c *stubChannel) Receive(ctx context.Context, sub *channels.Subscriber, data json.RawMessage) error {
	c.receivedCalls = append(c.receivedCalls, data)
	return nil
}

func TestSubscribeCallsChannelSubscribedWithParams(t *testing.T) {
	reg := channels.NewRegistry()
	stub := &stubChannel{}
	reg.Register("stub", func() channels.Channel { return stub })

	hub := channels.NewHub(memory.New(), reg)
	conn := hub.Connect()

	err := hub.Subscribe(context.Background(), conn.ID(), "stub", map[string]string{"room_id": "42"})
	require.NoError(t, err)
	require.Len(t, stub.subscribedCalls, 1)
	require.Equal(t, "42", stub.subscribedCalls[0]["room_id"])
}

func TestSubscribeReturnsErrUnknownChannel(t *testing.T) {
	hub := channels.NewHub(memory.New(), channels.NewRegistry())
	conn := hub.Connect()

	err := hub.Subscribe(context.Background(), conn.ID(), "missing", nil)
	require.ErrorIs(t, err, channels.ErrUnknownChannel)
}

func TestSubscribeReturnsErrUnknownConnection(t *testing.T) {
	reg := channels.NewRegistry()
	reg.Register("stub", func() channels.Channel { return &stubChannel{} })

	hub := channels.NewHub(memory.New(), reg)

	err := hub.Subscribe(context.Background(), "nope", "stub", nil)
	require.ErrorIs(t, err, channels.ErrUnknownConnection)
}

func TestSubscribeTwiceOnSameChannelReturnsErrAlreadySubscribed(t *testing.T) {
	reg := channels.NewRegistry()
	reg.Register("stub", func() channels.Channel { return &stubChannel{} })

	hub := channels.NewHub(memory.New(), reg)
	conn := hub.Connect()

	require.NoError(t, hub.Subscribe(context.Background(), conn.ID(), "stub", nil))
	err := hub.Subscribe(context.Background(), conn.ID(), "stub", nil)
	require.ErrorIs(t, err, channels.ErrAlreadySubscribed)
}

func TestSubscribedErrorPreventsSubscription(t *testing.T) {
	boom := errors.New("boom")
	stub := &stubChannel{subscribeErr: boom}
	reg := channels.NewRegistry()
	reg.Register("chat", func() channels.Channel { return stub })

	hub := channels.NewHub(memory.New(), reg)
	conn := hub.Connect()

	err := hub.Subscribe(context.Background(), conn.ID(), "chat", nil)
	require.ErrorIs(t, err, boom)

	err = hub.Receive(context.Background(), conn.ID(), "chat", nil)
	require.ErrorIs(t, err, channels.ErrNotSubscribed)
}

func TestBroadcastDeliversToStreamingSubscriber(t *testing.T) {
	stub := &stubChannel{streamTopic: "room_42"}
	reg := channels.NewRegistry()
	reg.Register("chat", func() channels.Channel { return stub })

	hub := channels.NewHub(memory.New(), reg)
	conn := hub.Connect()
	require.NoError(t, hub.Subscribe(context.Background(), conn.ID(), "chat", nil))

	require.NoError(t, hub.Broadcast(context.Background(), "room_42", []byte("hi")))

	select {
	case msg := <-conn.Outbox():
		require.Equal(t, "chat", msg.Channel)
		require.Equal(t, []byte("hi"), msg.Payload)
	case <-time.After(time.Second):
		t.Fatal("expected a delivered message")
	}
}

func TestBroadcastFansOutToMultipleConnections(t *testing.T) {
	reg := channels.NewRegistry()
	reg.Register("chat", func() channels.Channel { return &stubChannel{streamTopic: "room_42"} })

	hub := channels.NewHub(memory.New(), reg)
	connA := hub.Connect()
	connB := hub.Connect()
	require.NoError(t, hub.Subscribe(context.Background(), connA.ID(), "chat", nil))
	require.NoError(t, hub.Subscribe(context.Background(), connB.ID(), "chat", nil))

	require.NoError(t, hub.Broadcast(context.Background(), "room_42", []byte("hi")))

	for _, conn := range []*channels.Conn{connA, connB} {
		select {
		case msg := <-conn.Outbox():
			require.Equal(t, []byte("hi"), msg.Payload)
		case <-time.After(time.Second):
			t.Fatalf("expected a delivered message for conn %s", conn.ID())
		}
	}
}

func TestReceiveDispatchesToChannelReceive(t *testing.T) {
	stub := &stubChannel{}
	reg := channels.NewRegistry()
	reg.Register("chat", func() channels.Channel { return stub })

	hub := channels.NewHub(memory.New(), reg)
	conn := hub.Connect()
	require.NoError(t, hub.Subscribe(context.Background(), conn.ID(), "chat", nil))

	require.NoError(t, hub.Receive(context.Background(), conn.ID(), "chat", json.RawMessage(`{"message":"hi"}`)))
	require.Len(t, stub.receivedCalls, 1)
	require.JSONEq(t, `{"message":"hi"}`, string(stub.receivedCalls[0]))
}

func TestReceiveReturnsErrNotSubscribed(t *testing.T) {
	hub := channels.NewHub(memory.New(), channels.NewRegistry())
	conn := hub.Connect()

	err := hub.Receive(context.Background(), conn.ID(), "chat", nil)
	require.ErrorIs(t, err, channels.ErrNotSubscribed)
}

func TestReceiveReturnsErrUnknownConnection(t *testing.T) {
	hub := channels.NewHub(memory.New(), channels.NewRegistry())

	err := hub.Receive(context.Background(), "nope", "chat", nil)
	require.ErrorIs(t, err, channels.ErrUnknownConnection)
}

func TestUnsubscribeCallsChannelUnsubscribedAndStopsDelivery(t *testing.T) {
	stub := &stubChannel{streamTopic: "room_42"}
	reg := channels.NewRegistry()
	reg.Register("chat", func() channels.Channel { return stub })

	hub := channels.NewHub(memory.New(), reg)
	conn := hub.Connect()
	require.NoError(t, hub.Subscribe(context.Background(), conn.ID(), "chat", nil))

	require.NoError(t, hub.Unsubscribe(context.Background(), conn.ID(), "chat"))
	require.Equal(t, 1, stub.unsubscribedCalls)

	require.NoError(t, hub.Broadcast(context.Background(), "room_42", []byte("late")))

	select {
	case msg := <-conn.Outbox():
		t.Fatalf("expected no message after unsubscribe, got %+v", msg)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestUnsubscribeReturnsErrNotSubscribed(t *testing.T) {
	hub := channels.NewHub(memory.New(), channels.NewRegistry())
	conn := hub.Connect()

	err := hub.Unsubscribe(context.Background(), conn.ID(), "chat")
	require.ErrorIs(t, err, channels.ErrNotSubscribed)
}

func TestDisconnectTearsDownAllSubscriptions(t *testing.T) {
	stub := &stubChannel{streamTopic: "room_42"}
	reg := channels.NewRegistry()
	reg.Register("chat", func() channels.Channel { return stub })

	hub := channels.NewHub(memory.New(), reg)
	conn := hub.Connect()
	require.NoError(t, hub.Subscribe(context.Background(), conn.ID(), "chat", nil))

	hub.Disconnect(context.Background(), conn.ID())
	require.Equal(t, 1, stub.unsubscribedCalls)

	err := hub.Unsubscribe(context.Background(), conn.ID(), "chat")
	require.ErrorIs(t, err, channels.ErrUnknownConnection)
}

func TestDisconnectIsSafeForUnknownConnection(t *testing.T) {
	hub := channels.NewHub(memory.New(), channels.NewRegistry())
	hub.Disconnect(context.Background(), "nope") // must not panic
}

func TestConnectReturnsUniqueConnectionIDs(t *testing.T) {
	hub := channels.NewHub(memory.New(), channels.NewRegistry())

	a := hub.Connect()
	b := hub.Connect()

	require.NotEmpty(t, a.ID())
	require.NotEqual(t, a.ID(), b.ID())
}
