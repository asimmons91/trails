// Package database provides a cache.Store backed by a SQL table via pack,
// trails' ORM — the SolidCache equivalent for trails. Entries are upserted
// on Write (atomic per-row on every dialect via ON CONFLICT / ON DUPLICATE
// KEY UPDATE) and swept for expiry in batches by Run, which satisfies
// trails.Runner.
//
// Schema is not created automatically: register Migration in the app's own
// db/migrations package before using this backend (see Migration's doc
// comment).
package database

import (
	"context"
	"errors"
	"log/slog"
	"time"

	trails "github.com/asimmons91/trails"
	"github.com/asimmons91/trails/cache"
	"github.com/asimmons91/trails/pack"
)

const (
	defaultSweepInterval  = time.Minute
	defaultSweepBatchSize = 500
)

var (
	_ cache.Store   = (*Backend)(nil)
	_ trails.Runner = (*Backend)(nil)
)

type Option func(*Backend)

func WithSweepInterval(d time.Duration) Option {
	return func(b *Backend) { b.sweepInterval = d }
}

func WithSweepBatchSize(n int) Option {
	return func(b *Backend) { b.sweepBatchSize = n }
}

func WithLogger(l *slog.Logger) Option {
	return func(b *Backend) { b.logger = l }
}

type Backend struct {
	db *pack.DB

	sweepInterval  time.Duration
	sweepBatchSize int
	logger         *slog.Logger
}

func New(db *pack.DB, opts ...Option) *Backend {
	b := &Backend{
		db:             db,
		sweepInterval:  defaultSweepInterval,
		sweepBatchSize: defaultSweepBatchSize,
		logger:         slog.Default(),
	}

	for _, opt := range opts {
		opt(b)
	}

	return b
}

func (b *Backend) Read(ctx context.Context, key string) ([]byte, bool, error) {
	row, ok, err := b.lookup(ctx, key)
	if err != nil || !ok {
		return nil, false, err
	}
	return row.Value, true, nil
}

func (b *Backend) Write(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	now := time.Now()

	var expiresAt int64
	if ttl > 0 {
		expiresAt = now.Add(ttl).UnixNano()
	}

	return pack.Create(ctx, b.db, &entryRow{
		Key:       key,
		Value:     value,
		ExpiresAt: expiresAt,
		CreatedAt: now.UnixNano(),
	}, pack.OnConflict[entryRow](entryCol.Key).DoUpdate(
		entryCol.Value.Set(value),
		entryCol.ExpiresAt.Set(expiresAt),
	))
}

func (b *Backend) Delete(ctx context.Context, key string) error {
	_, err := pack.Of[entryRow](b.db).Where(entryCol.Key.Eq(key)).DeleteAll(ctx)
	return err
}

func (b *Backend) Exist(ctx context.Context, key string) (bool, error) {
	_, ok, err := b.lookup(ctx, key)
	return ok, err
}

func (b *Backend) Clear(ctx context.Context) error {
	_, err := pack.Of[entryRow](b.db).DeleteAll(ctx)
	return err
}

// lookup reads the row for key, treating a missing or already-expired row
// as a miss. An expired row is deleted lazily, best-effort, the same way
// cache/backend/memory evicts on read.
func (b *Backend) lookup(ctx context.Context, key string) (entryRow, bool, error) {
	row, err := pack.Of[entryRow](b.db).Where(entryCol.Key.Eq(key)).First(ctx)
	if err != nil {
		if errors.Is(err, pack.ErrNoRows) {
			return entryRow{}, false, nil
		}
		return entryRow{}, false, err
	}

	if row.ExpiresAt != 0 && row.ExpiresAt <= time.Now().UnixNano() {
		if _, err := pack.Of[entryRow](b.db).Where(entryCol.Key.Eq(key)).DeleteAll(ctx); err != nil {
			b.logger.Error("cache/database: evicting expired entry failed", "error", err)
		}
		return entryRow{}, false, nil
	}

	return row, true, nil
}
