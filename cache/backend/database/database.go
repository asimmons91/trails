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

// Option configures New.
type Option func(*Backend)

// WithSweepInterval sets how often Run sweeps expired entries. The
// default is one minute.
func WithSweepInterval(d time.Duration) Option {
	return func(b *Backend) { b.sweepInterval = d }
}

// WithSweepBatchSize sets how many expired rows Run deletes per batch.
// The default is 500.
func WithSweepBatchSize(n int) Option {
	return func(b *Backend) { b.sweepBatchSize = n }
}

// WithLogger sets the logger used to report background failures (a
// failed sweep, or a failed best-effort eviction on read). The default is
// slog.Default().
func WithLogger(l *slog.Logger) Option {
	return func(b *Backend) { b.logger = l }
}

// Backend is a cache.Store backed by a SQL table via pack. Construct one
// with New.
type Backend struct {
	db *pack.DB

	sweepInterval  time.Duration
	sweepBatchSize int
	logger         *slog.Logger
}

// New returns a ready-to-use Backend backed by db. Register Migration
// before first use (see Migration's doc comment).
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

// Read returns the value stored at key, and false if key is absent or has
// expired. An expired row is evicted, best-effort, as a side effect of
// this call.
func (b *Backend) Read(ctx context.Context, key string) ([]byte, bool, error) {
	row, ok, err := b.lookup(ctx, key)
	if err != nil || !ok {
		return nil, false, err
	}
	return row.Value, true, nil
}

// Write upserts value at key (atomic per-row on every dialect via ON
// CONFLICT / ON DUPLICATE KEY UPDATE), expiring after ttl (ttl <= 0 means
// never).
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

// Increment implements cache.Store's Increment contract by locking key's
// row and doing the arithmetic in Go rather than in SQL — Value is a
// BYTEA/BLOB column, and "value = value + delta" against blob storage
// needs brittle, dialect-specific casts. Concurrent callers incrementing
// the same key contend for that row, which under load can trip a
// deadlock/lock-timeout error (MySQL/InnoDB) or race to create the same
// new key (ErrUniqueViolation); Increment retries either, bounded, with
// jittered backoff.
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

// Delete removes key. It is not an error if key doesn't exist.
func (b *Backend) Delete(ctx context.Context, key string) error {
	_, err := pack.Of[entryRow](b.db).Where(entryCol.Key.Eq(key)).DeleteAll(ctx)
	return err
}

// Exist reports whether key is present and unexpired, evicting it,
// best-effort, as a side effect if it has expired.
func (b *Backend) Exist(ctx context.Context, key string) (bool, error) {
	_, ok, err := b.lookup(ctx, key)
	return ok, err
}

// Clear removes every entry.
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
