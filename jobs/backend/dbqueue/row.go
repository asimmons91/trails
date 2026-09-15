package dbqueue

import "github.com/asimmons91/trails/pack"

// Job states. A row moves available -> executing -> finished, or back to
// available (retry) / failed (attempts exhausted) on error.
const (
	stateAvailable = "available"
	stateExecuting = "executing"
	stateFinished  = "finished"
	stateFailed    = "failed"
)

const defaultQueue = "default"

// jobRow is the persisted form of a jobs.Enqueued. Args is stored as a plain
// string (the JSON text itself), not via pack's `type:jsonb` marshaling: that
// tag is rendered as a literal "JSONB" column type on every dialect, which
// SQLite has no type-affinity rule for (it falls through to NUMERIC
// affinity and corrupts the stored text). A plain string column maps to
// TEXT on all three dialects with no special-casing required.
//
// Timestamps are Unix nanoseconds (int64), not time.Time: SQLite maps
// time.Time to a bare TEXT column that doesn't scan back into time.Time,
// the same reason pack/migrate's schemaMigrationRow avoids it.
//
// No field is null_zero: pack's scan path only special-cases NULL for
// `type:jsonb` columns, so a null_zero column holding a real NULL fails to
// scan back into a plain string/int64 field ("converting NULL to string is
// unsupported"). Nothing here needs SQL NULL semantics — every check is
// against state or a literal zero/empty value — so every column is a
// plain not_null with a zero-value default instead (0 for unset timestamps,
// "" for unset strings, matching jobs.Enqueued's own "" = unlimited
// convention for ConcurrencyKey).
type jobRow struct {
	pack.Model[int64] `db:"table:dbqueue_jobs"`

	Kind                  string `db:"kind,not_null"`
	Args                  string `db:"args,not_null"`
	Queue                 string `db:"queue,not_null"`
	State                 string `db:"state,not_null"`
	ScheduledAt           int64  `db:"scheduled_at,not_null"`
	Attempts              int    `db:"attempts,not_null"`
	MaxAttempts           int    `db:"max_attempts,not_null"`
	ConcurrencyKey        string `db:"concurrency_key,not_null"`
	ConcurrencyLimit      int    `db:"concurrency_limit,not_null"`
	ConcurrencyDurationNs int64  `db:"concurrency_duration_ns,not_null"`
	LockedAt              int64  `db:"locked_at,not_null"`
	LockedBy              string `db:"locked_by,not_null"`
	LastError             string `db:"last_error,not_null"`
	FinishedAt            int64  `db:"finished_at,not_null"`
	CreatedAt             int64  `db:"created_at,not_null"`
}

var jobCol = struct {
	ID                    pack.Col[jobRow, int64]
	Kind                  pack.Col[jobRow, string]
	Args                  pack.Col[jobRow, string]
	Queue                 pack.Col[jobRow, string]
	State                 pack.Col[jobRow, string]
	ScheduledAt           pack.Col[jobRow, int64]
	Attempts              pack.Col[jobRow, int]
	MaxAttempts           pack.Col[jobRow, int]
	ConcurrencyKey        pack.Col[jobRow, string]
	ConcurrencyLimit      pack.Col[jobRow, int]
	ConcurrencyDurationNs pack.Col[jobRow, int64]
	LockedAt              pack.Col[jobRow, int64]
	LockedBy              pack.Col[jobRow, string]
	LastError             pack.Col[jobRow, string]
	FinishedAt            pack.Col[jobRow, int64]
	CreatedAt             pack.Col[jobRow, int64]
}{
	ID:                    pack.Field(func(r *jobRow) *int64 { return &r.ID }),
	Kind:                  pack.Field(func(r *jobRow) *string { return &r.Kind }),
	Args:                  pack.Field(func(r *jobRow) *string { return &r.Args }),
	Queue:                 pack.Field(func(r *jobRow) *string { return &r.Queue }),
	State:                 pack.Field(func(r *jobRow) *string { return &r.State }),
	ScheduledAt:           pack.Field(func(r *jobRow) *int64 { return &r.ScheduledAt }),
	Attempts:              pack.Field(func(r *jobRow) *int { return &r.Attempts }),
	MaxAttempts:           pack.Field(func(r *jobRow) *int { return &r.MaxAttempts }),
	ConcurrencyKey:        pack.Field(func(r *jobRow) *string { return &r.ConcurrencyKey }),
	ConcurrencyLimit:      pack.Field(func(r *jobRow) *int { return &r.ConcurrencyLimit }),
	ConcurrencyDurationNs: pack.Field(func(r *jobRow) *int64 { return &r.ConcurrencyDurationNs }),
	LockedAt:              pack.Field(func(r *jobRow) *int64 { return &r.LockedAt }),
	LockedBy:              pack.Field(func(r *jobRow) *string { return &r.LockedBy }),
	LastError:             pack.Field(func(r *jobRow) *string { return &r.LastError }),
	FinishedAt:            pack.Field(func(r *jobRow) *int64 { return &r.FinishedAt }),
	CreatedAt:             pack.Field(func(r *jobRow) *int64 { return &r.CreatedAt }),
}
