// Package memory provides an in-process, non-durable channels.Broadcaster:
// Publish delivers synchronously to every current subscriber within the
// same process, and Close closes every subscriber's Messages channel and
// makes the Backend permanently unusable. Its Run is a no-op (there's
// nothing to poll) — it exists only so memory.Backend is interchangeable
// with channels/backend/database wherever a trails.Runner is expected.
package memory

import (
	"context"
	"errors"
	"sync"

	trails "github.com/asimmons91/trails"
	"github.com/asimmons91/trails/channels"
)

const defaultSubscriptionBuffer = 16

// ErrClosed is returned by Publish and Subscribe once the Backend has
// been Closed.
var ErrClosed = errors.New("memory: broadcaster closed")

var (
	_ channels.Broadcaster = (*Backend)(nil)
	_ trails.Runner        = (*Backend)(nil)
)

// Backend is an in-process channels.Broadcaster. Construct one with New.
type Backend struct {
	mu     sync.Mutex
	topics map[string]map[*subscription]struct{}
	closed bool
}

// New returns a ready-to-use Backend.
func New() *Backend {
	return &Backend{topics: make(map[string]map[*subscription]struct{})}
}

// Publish delivers payload synchronously to every current subscriber of
// topic within this process. A subscriber whose buffer is full is
// dropped rather than blocking the publisher or other subscribers.
func (b *Backend) Publish(_ context.Context, topic string, payload []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return ErrClosed
	}

	for sub := range b.topics[topic] {
		select {
		case sub.messages <- payload:
		default:
			// slow subscriber: drop rather than block the publisher
		}
	}
	return nil
}

// Subscribe returns a Subscription that receives every payload
// subsequently Published to topic in this process.
func (b *Backend) Subscribe(_ context.Context, topic string) (channels.Subscription, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil, ErrClosed
	}

	sub := &subscription{
		backend:  b,
		topic:    topic,
		messages: make(chan []byte, defaultSubscriptionBuffer),
	}
	if b.topics[topic] == nil {
		b.topics[topic] = make(map[*subscription]struct{})
	}
	b.topics[topic][sub] = struct{}{}

	return sub, nil
}

// Run is a no-op: memory.Backend delivers synchronously in Publish and has
// nothing to poll. It exists so memory.Backend is interchangeable with
// database.Backend wherever a trails.Runner is expected.
func (b *Backend) Run(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

// Close closes every current Subscription's Messages channel and makes b
// permanently unusable — subsequent Publish/Subscribe calls return
// ErrClosed. It is safe to call more than once.
func (b *Backend) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true

	var subs []*subscription
	for _, set := range b.topics {
		for sub := range set {
			subs = append(subs, sub)
		}
	}
	b.topics = make(map[string]map[*subscription]struct{})
	b.mu.Unlock()

	for _, sub := range subs {
		_ = sub.Unsubscribe()
	}
	return nil
}

type subscription struct {
	backend  *Backend
	topic    string
	messages chan []byte
	once     sync.Once
}

var _ channels.Subscription = (*subscription)(nil)

func (s *subscription) Messages() <-chan []byte { return s.messages }

func (s *subscription) Unsubscribe() error {
	s.once.Do(func() {
		s.backend.mu.Lock()
		if subs, ok := s.backend.topics[s.topic]; ok {
			delete(subs, s)
			if len(subs) == 0 {
				delete(s.backend.topics, s.topic)
			}
		}
		s.backend.mu.Unlock()
		close(s.messages)
	})
	return nil
}
