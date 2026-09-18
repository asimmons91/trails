// Package database provides a channels.Broadcaster backed by a SQL table
// via pack, trails' ORM: Publish inserts a row, and Run polls for rows
// newer than the last one it saw and delivers them to local subscribers,
// so messages reach every process running Run against the same table, not
// just the process that Published them. Run also periodically trims rows
// older than the configured retention window.
//
// Schema is not created automatically: register Migration in the app's
// own db/migrations package before using this backend (see Migration's
// doc comment).
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

// ErrClosed is returned by Publish and Subscribe once the Backend has
// been Closed.
var ErrClosed = errors.New("database: broadcaster closed")

var (
	_ channels.Broadcaster = (*Backend)(nil)
	_ trails.Runner        = (*Backend)(nil)
)

// Option configures New.
type Option func(*Backend)

// WithPollInterval sets how often Run polls for new messages. The
// default is 250ms.
func WithPollInterval(d time.Duration) Option {
	return func(b *Backend) { b.pollInterval = d }
}

// WithBatchSize sets how many rows Run's poll fetches per query (it
// keeps querying in batches of this size until a batch comes back
// short). The default is 100.
func WithBatchSize(n int) Option {
	return func(b *Backend) { b.batchSize = n }
}

// WithRetention sets how long a message row is kept before Run's trim
// deletes it. The default is 5 minutes.
func WithRetention(d time.Duration) Option {
	return func(b *Backend) { b.retention = d }
}

// WithTrimInterval sets how often Run trims rows older than retention.
// The default is one minute.
func WithTrimInterval(d time.Duration) Option {
	return func(b *Backend) { b.trimInterval = d }
}

// WithTrimBatchSize sets how many stale rows Run's trim deletes per
// batch. The default is 500.
func WithTrimBatchSize(n int) Option {
	return func(b *Backend) { b.trimBatchSize = n }
}

// WithLogger sets the logger used to report a failed poll or trim. The
// default is slog.Default().
func WithLogger(l *slog.Logger) Option {
	return func(b *Backend) { b.logger = l }
}

// Backend is a channels.Broadcaster backed by a SQL table via pack.
// Construct one with New.
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

// New returns a ready-to-use Backend backed by db, with its delivery
// cursor seeded at the newest existing row — messages published before
// New was called are never delivered to subscribers of this Backend.
// Register Migration before first use (see Migration's doc comment).
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

// Publish inserts a row recording payload on topic. Delivery to local
// subscribers happens later, when Run's poll loop reaches that row — not
// synchronously within Publish.
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

// Subscribe returns a Subscription that receives every payload Published
// to topic from the moment Run's poll loop next reaches it onward.
// Subscribe itself never reads the database, and does nothing without a
// running Run.
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

// Close closes every current Subscription's Messages channel and makes b
// permanently unusable — subsequent Publish/Subscribe calls return
// ErrClosed. It does not stop Run; cancel Run's context separately. It is
// safe to call more than once.
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
