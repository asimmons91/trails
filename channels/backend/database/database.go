package database

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/asimmons91/trails"
	"github.com/asimmons91/trails/channels"
	"github.com/asimmons91/trails/pack"
)

const (
	defaultPollInterval  = 250 * time.Millisecond
	defaultBatchSize     = 100
	defaultRetention     = 5 * time.Minute
	defaultTrimInterval  = time.Minute
	defaultTrimBatchSize = 500

	defaultSubscriptionBuffer = 16
)

var ErrClosed = errors.New("database: broadcaster closed")

var (
	_ channels.Broadcaster = (*Backend)(nil)
	_ trails.Runner        = (*Backend)(nil)
)

type Option func(*Backend)

func WithPollInterval(d time.Duration) Option {
	return func(b *Backend) { b.pollInterval = d }
}

func WithBatchSize(n int) Option {
	return func(b *Backend) { b.batchSize = n }
}

func WithRetention(d time.Duration) Option {
	return func(b *Backend) { b.retention = d }
}

func WithTrimInterval(d time.Duration) Option {
	return func(b *Backend) { b.trimInterval = d }
}

func WithTrimBatchSize(n int) Option {
	return func(b *Backend) { b.trimBatchSize = n }
}

func WithLogger(l *slog.Logger) Option {
	return func(b *Backend) { b.logger = l }
}

type Backend struct {
	db *pack.DB

	mu     sync.Mutex
	subs   map[string]map[*subscription]struct{}
	cursor int64
	closed bool

	pollInterval, trimInterval time.Duration
	batchSize, trimBatchSize   int
	retention                  time.Duration
	logger                     *slog.Logger
}

func New(ctx context.Context, db *pack.DB, opts ...Option) (*Backend, error) {
	b := &Backend{
		db:            db,
		subs:          make(map[string]map[*subscription]struct{}),
		pollInterval:  defaultPollInterval,
		batchSize:     defaultBatchSize,
		retention:     defaultRetention,
		trimInterval:  defaultTrimInterval,
		trimBatchSize: defaultTrimBatchSize,
		logger:        slog.Default(),
	}

	for _, opt := range opts {
		opt(b)
	}

	latest, err := pack.Of[messageRow](db).Order(messageCol.ID.Desc()).Limit(1).First(ctx)
	if err != nil && !errors.Is(err, pack.ErrNoRows) {
		return nil, err
	}
	b.cursor = latest.ID

	return b, nil
}

func (b *Backend) Publish(ctx context.Context, topic string, payload []byte) error {
	b.mu.Lock()
	closed := b.closed
	b.mu.Unlock()
	if closed {
		return ErrClosed
	}

	row := &messageRow{
		Topic:     topic,
		Payload:   string(payload),
		CreatedAt: time.Now().UnixNano(),
	}
	return pack.Create(ctx, b.db, row)
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
	if b.subs[topic] == nil {
		b.subs[topic] = make(map[*subscription]struct{})
	}
	b.subs[topic][sub] = struct{}{}

	return sub, nil
}

func (b *Backend) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true

	var subs []*subscription
	for _, set := range b.subs {
		for sub := range set {
			subs = append(subs, sub)
		}
	}
	b.subs = make(map[string]map[*subscription]struct{})
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
		if subs, ok := s.backend.subs[s.topic]; ok {
			delete(subs, s)
			if len(subs) == 0 {
				delete(s.backend.subs, s.topic)
			}
		}
		s.backend.mu.Unlock()
		close(s.messages)
	})
	return nil
}
