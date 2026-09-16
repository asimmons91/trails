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
	ErrUnknownConnection = errors.New("channels: unknown connection")
	ErrAlreadySubscribed = errors.New("channels: already subscribed")
	ErrNotSubscribed     = errors.New("channels: not subscribed")
)

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

// Unsubscribe tears down one channel subscription on one connection.
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

func (h *Hub) Broadcast(ctx context.Context, topic string, payload []byte) error {
	return h.broadcaster.Publish(ctx, topic, payload)
}

func newConnID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
