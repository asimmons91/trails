package pack

import "github.com/asimmons91/trails/pack/internal/sqlbuild"

// WriteOption configures a Create or CreateAll call. Build one with
// WithBatchSize or OnConflict.
type WriteOption interface {
	applyToWriteConfig(c *writeConfig)
}

type conflictSpec struct {
	cols      []sqlbuild.Column
	doNothing bool
	doUpdate  []sqlbuild.Assignment
}

type writeConfig struct {
	batchSize int
	conflict  *conflictSpec
}

type batchSizeOption struct{ n int }

func (b batchSizeOption) applyToWriteConfig(c *writeConfig) {
	c.batchSize = b.n
}

// WithBatchSize overrides CreateAll's default number of rows per INSERT
// statement (defaultCreateAllBatchSize).
func WithBatchSize(n int) WriteOption {
	return batchSizeOption{n}
}

type conflictOption struct{ spec conflictSpec }

func (o conflictOption) applyToWriteConfig(c *writeConfig) {
	c.conflict = &o.spec
}

// ConflictBuilder[T] names the columns an upsert conflicts on; chain
// DoNothing or DoUpdate to finish it into a WriteOption. Build one with
// OnConflict.
type ConflictBuilder[T any] struct {
	cols []AnyCol[T]
}

// OnConflict starts a ConflictBuilder for an upsert conflicting on cols
// (e.g. a unique index's columns). See ExampleOnConflict.
func OnConflict[T any](cols ...AnyCol[T]) *ConflictBuilder[T] {
	return &ConflictBuilder[T]{cols}
}

func (c *ConflictBuilder[T]) sqlCols() []sqlbuild.Column {
	cols := make([]sqlbuild.Column, len(c.cols))
	for i, c := range c.cols {
		cols[i] = c.sqlColumn()
	}

	return cols
}

// DoNothing finishes the option so a conflicting row is left unchanged
// (ON CONFLICT ... DO NOTHING, or the dialect's equivalent, e.g. MySQL's
// INSERT IGNORE).
func (c *ConflictBuilder[T]) DoNothing() WriteOption {
	return conflictOption{spec: conflictSpec{cols: c.sqlCols(), doNothing: true}}
}

// DoUpdate finishes the option so a conflicting row is updated with
// assignments instead (ON CONFLICT ... DO UPDATE SET ..., or the
// dialect's equivalent, e.g. MySQL's ON DUPLICATE KEY UPDATE).
func (c *ConflictBuilder[T]) DoUpdate(assignments ...Assignment) WriteOption {
	raw := make([]sqlbuild.Assignment, len(assignments))
	for i, a := range assignments {
		raw[i] = a.a
	}

	return conflictOption{spec: conflictSpec{cols: c.sqlCols(), doUpdate: raw}}
}
