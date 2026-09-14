package pack

import "github.com/asimmons91/trails/pack/internal/sqlbuild"

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

func WithBatchSize(n int) WriteOption {
	return batchSizeOption{n}
}

type conflictOption struct{ spec conflictSpec }

func (o conflictOption) applyToWriteConfig(c *writeConfig) {
	c.conflict = &o.spec
}

type ConflictBuilder[T any] struct {
	cols []AnyCol[T]
}

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

func (c *ConflictBuilder[T]) DoNothing() WriteOption {
	return conflictOption{spec: conflictSpec{cols: c.sqlCols(), doNothing: true}}
}

func (c *ConflictBuilder[T]) DoUpdate(assignments ...Assignment) WriteOption {
	raw := make([]sqlbuild.Assignment, len(assignments))
	for i, a := range assignments {
		raw[i] = a.a
	}

	return conflictOption{spec: conflictSpec{cols: c.sqlCols(), doUpdate: raw}}
}
