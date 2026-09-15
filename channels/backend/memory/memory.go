package memory

import (
	"context"
	"errors"
	"sync"

	trails "github.com/asimmons91/trails"
	"github.com/asimmons91/trails/channels"
)

const defaultSubscriptionBuffer = 16

var ErrClosed = errors.New("memory: broadcaster closed")

var (
	_ channels.Broadcaster = (*Backend)(nil)
	_ trails.Runner        = (*Backend)(nil)
)

type Backend struct {
	mu     sync.Mutex
	topics map[string]map[*subscription]struct{}
	closed bool
}

func New() *Backend {
	return &Backend{topics: make(map[string]map[*subscription]struct{})}
}

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
