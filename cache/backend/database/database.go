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
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strconv"
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

// maxIncrementAttempts bounds retries of an Increment transaction lost to
// contention on the same row. Many concurrent callers locking the exact
// same key (unlike dbqueue's SkipLocked, which sidesteps contention by
// skipping to a different row instead) makes an occasional deadlock/lock
// timeout, or two callers racing to create the same brand-new key, expected
// under load, not exceptional — MySQL's own error text for the former says
// to retry ("try restarting transaction"), so Increment does, for both.
const maxIncrementAttempts = 50

// incrementRetryBackoff is a small jittered delay between retries, so a
// burst of callers that all collided on the same attempt don't immediately
// collide again on the next one.
func incrementRetryBackoff(attempt int) time.Duration {
	return time.Duration(1+rand.IntN(4)) * time.Millisecond * time.Duration(attempt)
}

// Increment atomically adds delta to the integer counter stored at key,
// reusing entryRow/entryCol as-is (no schema change). This intentionally
// does not do the arithmetic in SQL: Value is a []byte (BYTEA/BLOB) column,
// and doing "value = value + delta" against that column's storage requires
// brittle, dialect-specific CAST idioms to get in and back out of blob
// storage — doing the arithmetic in Go against an already-locked row avoids
// that entirely.
//
// Every caller locking the same key's row is exactly the kind of hot-row
// contention that can trip a database's deadlock detector, or two callers
// racing to create the same brand-new key — both observed in practice
// under MySQL/InnoDB when many transactions target the same key
// concurrently, even though there is only ever one row involved — so this
// retries a bounded number of times with jittered backoff before giving up.
func (b *Backend) Increment(ctx context.Context, key string, delta int64, ttl time.Duration) (int64, time.Time, error) {
	var (
		count     int64
		expiresAt time.Time
		err       error
	)

	for attempt := 0; attempt < maxIncrementAttempts; attempt++ {
		count, expiresAt, err = b.incrementOnce(ctx, key, delta, ttl)
		if err == nil {
			return count, expiresAt, nil
		}

		var serErr *pack.ErrSerializationFailure
		var uniqueErr *pack.ErrUniqueViolation
		if !errors.As(err, &serErr) && !errors.As(err, &uniqueErr) {
			return count, expiresAt, err
		}

		time.Sleep(incrementRetryBackoff(attempt + 1))
	}

	return count, expiresAt, err
}

// incrementOnce locks key's row (FOR UPDATE where the dialect supports row
// locking; SQLite doesn't, but its writers already serialize at the
// connection/database level, so the surrounding Tx alone is atomic there)
// if it exists, or creates it (a plain INSERT, not an upsert) if it
// doesn't, before computing and writing back the new value. Doing a plain
// SELECT ... FOR UPDATE followed by an UPDATE — rather than seeding every
// call with an INSERT ... ON DUPLICATE KEY UPDATE first — avoids a
// specific, well-known MySQL/InnoDB deadlock pattern: mixing an upsert
// against a unique secondary index with a subsequent FOR UPDATE read on
// the same row, across many concurrent transactions, deadlocks far more
// than either statement type alone. The plain INSERT here only runs once
// per key's lifetime (the first Increment after creation or expiry); if it
// loses a race to a concurrent creator, it fails with ErrUniqueViolation,
// which Increment's caller retries from scratch — by then the row exists,
// so the retry takes the lock-and-update path instead.
func (b *Backend) incrementOnce(ctx context.Context, key string, delta int64, ttl time.Duration) (int64, time.Time, error) {
	now := time.Now()

	var freshExpiresAt int64
	if ttl > 0 {
		freshExpiresAt = now.Add(ttl).UnixNano()
	}

	var result, resultExpiresAt int64

	err := b.db.Tx(ctx, func(tx *pack.DB) error {
		q := pack.Of[entryRow](tx).Where(entryCol.Key.Eq(key))
		if tx.Dialect().SupportsRowLocking() {
			q = q.ForUpdate()
		}
		row, err := q.First(ctx)

		switch {
		case errors.Is(err, pack.ErrNoRows):
			if err := pack.Create(ctx, tx, &entryRow{
				Key:       key,
				Value:     []byte(strconv.FormatInt(delta, 10)),
				ExpiresAt: freshExpiresAt,
				CreatedAt: now.UnixNano(),
			}); err != nil {
				return fmt.Errorf("cache/database: create increment row: %w", err)
			}
			result, resultExpiresAt = delta, freshExpiresAt
			return nil
		case err != nil:
			return fmt.Errorf("cache/database: lock increment row: %w", err)
		}

		var next, expiresAt int64
		if row.ExpiresAt != 0 && row.ExpiresAt <= now.UnixNano() {
			next, expiresAt = delta, freshExpiresAt
		} else {
			cur, perr := strconv.ParseInt(string(row.Value), 10, 64)
			if perr != nil {
				return fmt.Errorf("cache/database: increment %q: stored value is not an integer counter: %w", key, perr)
			}
			next, expiresAt = cur+delta, row.ExpiresAt
		}

		if _, err := pack.Of[entryRow](tx).Where(entryCol.Key.Eq(key)).Update(ctx,
			entryCol.Value.Set([]byte(strconv.FormatInt(next, 10))),
			entryCol.ExpiresAt.Set(expiresAt),
		); err != nil {
			return fmt.Errorf("cache/database: update increment row: %w", err)
		}

		result, resultExpiresAt = next, expiresAt
		return nil
	})
	if err != nil {
		return 0, time.Time{}, err
	}

	var expiresAt time.Time
	if resultExpiresAt != 0 {
		expiresAt = time.Unix(0, resultExpiresAt)
	}
	return result, expiresAt, nil
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
