// Package channels provides a Client/Server realtime pubsub system.
// Apps implement Channel and register a factory for each one in a
// Registry; a Hub ties that Registry to a pluggable Broadcaster, which
// fans messages out across connections and, depending on the
// implementation, across processes; channels.New (see spur.go) wraps a
// Hub as a trails.Spur that exposes it to clients over Server-Sent
// Events.
//
//	reg := channels.NewRegistry()
//	reg.Register("room", func() channels.Channel { return &RoomChannel{} })
//	hub := channels.NewHub(memory.New(), reg)
//	mounts := []trails.Mount{{Prefix: "/cable", Spur: channels.New(hub)}}
//
// A client opens the stream with a GET to the mount's root and receives a
// "connected" SSE event carrying a connection_id; every subsequent
// subscribe/unsubscribe/message command is a POST to the mount's command
// path (see Backend.Routes), carrying that ID in the HeaderConnectionID
// header. channels/backend/memory and channels/backend/database are the
// two Broadcaster implementations trails ships — memory delivers
// synchronously within one process, database polls a table so delivery
// also reaches other processes, at the cost of needing Backend.Run kept
// running (see RegisterSpurRunners).
package channels

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

const defaultOutboxSize = 16

var (
	// ErrUnknownConnection is returned, wrapped, by Hub.Subscribe,
	// Unsubscribe, and Receive when connID names a connection Hub doesn't
	// have (never connected, or already disconnected).
	ErrUnknownConnection = errors.New("channels: unknown connection")
	// ErrAlreadySubscribed is returned, wrapped, by Hub.Subscribe when the
	// connection already has a subscription under that channel name.
	ErrAlreadySubscribed = errors.New("channels: already subscribed")
	// ErrNotSubscribed is returned, wrapped, by Hub.Unsubscribe and
	// Receive when the connection has no subscription under that channel
	// name.
	ErrNotSubscribed = errors.New("channels: not subscribed")
)

// Hub is the runtime registry of live connections and their channel
// subscriptions. Construct one with NewHub; all of Hub's methods are safe
// for concurrent use.
type Hub struct {
	broadcaster Broadcaster
	registry    *Registry

	mu     sync.Mutex
	conns  map[string]*Conn
	topics map[string]*topicFanout
}

type topicFanout struct {
	sub         Subscription
	subscribers map[*Subscriber]struct{}
}

// NewHub returns a Hub that fans messages out via b and dispatches
// subscribe/message commands to channels registered in r.
func NewHub(b Broadcaster, r *Registry) *Hub {
	return &Hub{
		broadcaster: b,
		registry:    r,
		conns:       make(map[string]*Conn),
		topics:      make(map[string]*topicFanout),
	}
}

// Broadcaster returns the Hub's underlying Broadcaster, e.g. so a caller can
// check whether it needs a background Run loop started (see channels.Backend.Run).
func (h *Hub) Broadcaster() Broadcaster { return h.broadcaster }

// Connect registers a new, subscription-less connection and returns it.
// Callers are expected to eventually call Disconnect (typically when the
// transport serving it closes, see Backend.Routes).
func (h *Hub) Connect() *Conn {
	c := &Conn{
		id:     newConnID(),
		outbox: make(chan Message, defaultOutboxSize),
		subs:   make(map[string]*Subscriber),
	}

	h.mu.Lock()
	h.conns[c.id] = c
	h.mu.Unlock()

	return c
}

// Disconnect removes connID's connection and tears down every channel
// subscription it held, calling each Channel's Unsubscribed. It is a
// no-op if connID is unknown (e.g. already disconnected).
func (h *Hub) Disconnect(ctx context.Context, connID string) {
	h.mu.Lock()
	c, ok := h.conns[connID]
	if !ok {
		h.mu.Unlock()
		return
	}
	delete(h.conns, connID)

	subs := make([]*Subscriber, 0, len(c.subs))
	for _, sub := range c.subs {
		subs = append(subs, sub)
	}
	c.subs = make(map[string]*Subscriber)
	h.mu.Unlock()

	for _, sub := range subs {
		h.teardown(ctx, sub)
	}
}

// Subscribe creates a subscription to channelName on connID's connection,
// constructing a fresh Channel from the Registry and calling its
// Subscribed. It returns ErrUnknownConnection, ErrUnknownChannel, or
// ErrAlreadySubscribed (wrapped) without calling Subscribed; if
// Subscribed itself returns an error, the subscription is rolled back
// and that error is returned as-is.
func (h *Hub) Subscribe(ctx context.Context, connID, channelName string, params map[string]string) error {
	h.mu.Lock()
	c, ok := h.conns[connID]
	if !ok {
		h.mu.Unlock()
		return fmt.Errorf("%w: %q", ErrUnknownConnection, connID)
	}
	if _, exists := c.subs[channelName]; exists {
		h.mu.Unlock()
		return fmt.Errorf("%w: %q", ErrAlreadySubscribed, channelName)
	}
	h.mu.Unlock()

	ch, err := h.registry.new(channelName)
	if err != nil {
		return err
	}

	h.mu.Lock()
	// Re-check under lock: another Subscribe call for the same connection
	// and channel name may have raced in while the registry lookup above
	// ran unlocked.
	if _, exists := c.subs[channelName]; exists {
		h.mu.Unlock()
		return fmt.Errorf("%w: %q", ErrAlreadySubscribed, channelName)
	}

	sub := &Subscriber{
		conn:    c,
		channel: ch,
		name:    channelName,
		params:  params,
		hub:     h,
		topics:  make(map[string]struct{}),
	}
	c.subs[channelName] = sub
	h.mu.Unlock()

	if err := ch.Subscribed(ctx, sub, params); err != nil {
		h.mu.Lock()
		delete(c.subs, channelName)
		h.mu.Unlock()
		h.untrackTopics(sub)
		return err
	}

	return nil
}

// Unsubscribe tears down channelName's subscription on connID's
// connection, calling the Channel's Unsubscribed. It returns
// ErrUnknownConnection or ErrNotSubscribed (wrapped) if there is nothing
// to tear down.
func (h *Hub) Unsubscribe(ctx context.Context, connID, channelName string) error {
	h.mu.Lock()
	c, ok := h.conns[connID]
	if !ok {
		h.mu.Unlock()
		return fmt.Errorf("%w: %q", ErrUnknownConnection, connID)
	}
	sub, ok := c.subs[channelName]
	if !ok {
		h.mu.Unlock()
		return fmt.Errorf("%w: %q", ErrNotSubscribed, channelName)
	}
	delete(c.subs, channelName)
	h.mu.Unlock()

	h.teardown(ctx, sub)
	return nil
}

func (h *Hub) teardown(ctx context.Context, sub *Subscriber) {
	sub.channel.Unsubscribed(ctx, sub)
	h.untrackTopics(sub)
}

// Receive dispatches data to channelName's Channel.Receive on connID's
// connection. It returns ErrUnknownConnection or ErrNotSubscribed
// (wrapped) if there is no such subscription; any error Channel.Receive
// itself returns is returned as-is.
func (h *Hub) Receive(ctx context.Context, connID, channelName string, data json.RawMessage) error {
	h.mu.Lock()
	c, ok := h.conns[connID]
	if !ok {
		h.mu.Unlock()
		return fmt.Errorf("%w: %q", ErrUnknownConnection, connID)
	}
	sub, ok := c.subs[channelName]
	h.mu.Unlock()
	if !ok {
		return fmt.Errorf("%w: %q", ErrNotSubscribed, channelName)
	}

	return sub.channel.Receive(ctx, sub, data)
}

func (h *Hub) streamFrom(_ context.Context, sub *Subscriber, topic string) error {
	h.mu.Lock()
	tf, exists := h.topics[topic]
	if !exists {
		bsub, err := h.broadcaster.Subscribe(context.Background(), topic)
		if err != nil {
			h.mu.Unlock()
			return fmt.Errorf("channels: subscribing to topic %q: %w", topic, err)
		}
		tf = &topicFanout{sub: bsub, subscribers: make(map[*Subscriber]struct{})}
		h.topics[topic] = tf
		go h.pump(tf)
	}
	tf.subscribers[sub] = struct{}{}
	sub.topics[topic] = struct{}{}
	h.mu.Unlock()

	return nil
}

func (h *Hub) pump(tf *topicFanout) {
	for payload := range tf.sub.Messages() {
		h.mu.Lock()
		recipients := make([]*Subscriber, 0, len(tf.subscribers))
		for s := range tf.subscribers {
			recipients = append(recipients, s)
		}
		h.mu.Unlock()

		for _, s := range recipients {
			select {
			case s.conn.outbox <- Message{Channel: s.name, Payload: payload}:
			default:
				// slow consumer: drop rather than block the fan-out pump
			}
		}
	}
}

func (h *Hub) untrackTopics(sub *Subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for topic := range sub.topics {
		tf, ok := h.topics[topic]
		if !ok {
			continue
		}
		delete(tf.subscribers, sub)
		if len(tf.subscribers) == 0 {
			_ = tf.sub.Unsubscribe()
			delete(h.topics, topic)
		}
	}
	sub.topics = make(map[string]struct{})
}

// Broadcast publishes payload on topic via the Hub's Broadcaster,
// delivering it to every Subscriber currently streaming from topic (via
// StreamFrom) across every connection — including, depending on the
// Broadcaster, other processes. A recipient whose Outbox is full is
// dropped rather than blocking delivery to everyone else.
func (h *Hub) Broadcast(ctx context.Context, topic string, payload []byte) error {
	return h.broadcaster.Publish(ctx, topic, payload)
}

func newConnID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
