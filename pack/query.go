package pack

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"iter"
	"reflect"

	"github.com/asimmons91/trails/pack/internal/scan"
	"github.com/asimmons91/trails/pack/internal/schema"
	"github.com/asimmons91/trails/pack/internal/sqlbuild"
)

// Query[T] is a chainable, immutable query builder for model T, produced
// by Of. Every chain method returns a new *Query[T] (see clone) rather
// than mutating the receiver, so a partially-built Query can be safely
// branched and reused. Run it with Find, Rows, First, Count, or Exists, or
// use it as a set-based Update/Delete target.
type Query[T any] struct {
	db    *DB
	table *schema.Table

	sqlTable   sqlbuild.Table
	joins      []queryJoin
	selectCols []AnyCol[T]
	selectRaw  []string
	distinct   bool
	where      sqlbuild.Predicate
	having     sqlbuild.Predicate
	groupBy    []AnyCol[T]
	order      []sqlbuild.OrderTerm
	limit      *int64
	offset     *int64
	forUpdate  bool
	forShare   bool
	skipLocked bool
	preloads   []preloadRunner
	skipHooks  bool
}

// Of starts a query against T's mapped table. Panics if T isn't a valid
// pack model (no schema.For(T) mapping).
func Of[T any](db *DB) *Query[T] {
	t := reflect.TypeFor[T]()
	table, err := schema.For(t)
	if err != nil {
		panic(fmt.Sprintf("pack: Of[%s]: %v", t.Name(), err))
	}

	return &Query[T]{
		db:    db,
		table: table,
		sqlTable: sqlbuild.Table{
			Name:  table.Name,
			Alias: table.Alias,
		},
	}
}

// clone copies q so a chain method can mutate the copy, keeping every
// *Query[T] value returned so far unmodified (copy-on-write).
func (q *Query[T]) clone() *Query[T] {
	nq := *q
	return &nq
}

// Where AND-s p onto the query's existing WHERE condition, if any.
func (q *Query[T]) Where(p Predicate) *Query[T] {
	nq := q.clone()
	nq.where = andJoin(nq.where, p.p)
	return nq
}

// WhereRaw AND-s a raw SQL fragment onto the WHERE condition, with $-numbered
// placeholders in fragment bound to args.
func (q *Query[T]) WhereRaw(fragment string, args ...any) *Query[T] {
	return q.Where(rawPredicate(fragment, args...))
}

// Or OR-s p onto everything accumulated by Where/Or so far — not just the
// most recent one — so `.Where(a).Where(b).Or(c)` produces `(a AND b) OR
// c`, not `a AND (b OR c)`.
func (q *Query[T]) Or(p Predicate) *Query[T] {
	nq := q.clone()
	nq.where = orJoin(nq.where, p.p)
	return nq
}

// Scope is a reusable, composable query modifier, applied via Query[T].Scopes.
type Scope[T any] func(*Query[T]) *Query[T]

// Scopes applies each of scopes to q in order, threading the result of one
// into the next.
func (q *Query[T]) Scopes(scopes ...Scope[T]) *Query[T] {
	nq := q
	for _, s := range scopes {
		nq = s(nq)
	}
	return nq
}

// Order adds ORDER BY terms (see Col's Asc/Desc), appended after any
// already set.
func (q *Query[T]) Order(terms ...OrderTerm[T]) *Query[T] {
	nq := q.clone()
	raw := make([]sqlbuild.OrderTerm, len(terms))
	for i, t := range terms {
		raw[i] = t.o
	}
	nq.order = appendFresh(nq.order, raw...)
	return nq
}

// OrderRaw appends a raw ORDER BY term.
func (q *Query[T]) OrderRaw(fragment string) *Query[T] {
	nq := q.clone()
	nq.order = appendFresh(nq.order, sqlbuild.OrderRaw(fragment))
	return nq
}

// Limit sets LIMIT n.
func (q *Query[T]) Limit(n int64) *Query[T] {
	nq := q.clone()
	nq.limit = &n
	return nq
}

// Offset sets OFFSET n.
func (q *Query[T]) Offset(n int64) *Query[T] {
	nq := q.clone()
	nq.offset = &n
	return nq
}

// GroupBy adds GROUP BY columns, appended after any already set.
func (q *Query[T]) GroupBy(cols ...AnyCol[T]) *Query[T] {
	nq := q.clone()
	nq.groupBy = appendFresh(nq.groupBy, cols...)
	return nq
}

// Having AND-s p onto the query's existing HAVING condition, if any.
func (q *Query[T]) Having(p Predicate) *Query[T] {
	nq := q.clone()
	nq.having = andJoin(nq.having, p.p)
	return nq
}

// Distinct adds SELECT DISTINCT.
func (q *Query[T]) Distinct() *Query[T] {
	nq := q.clone()
	nq.distinct = true
	return nq
}

// Select restricts the columns fetched to cols instead of every mapped
// field, appended after any already set.
func (q *Query[T]) Select(cols ...AnyCol[T]) *Query[T] {
	nq := q.clone()
	nq.selectCols = appendFresh(nq.selectCols, cols...)
	return nq
}

// SelectRaw appends a raw SQL select expression.
func (q *Query[T]) SelectRaw(fragment string) *Query[T] {
	nq := q.clone()
	nq.selectRaw = appendFresh(nq.selectRaw, fragment)
	return nq
}

// ForUpdate adds a FOR UPDATE row-locking clause. Requires
// dialect.Dialect.SupportsRowLocking.
func (q *Query[T]) ForUpdate() *Query[T] {
	nq := q.clone()
	nq.forUpdate = true
	return nq
}

// ForShare adds a FOR SHARE row-locking clause. Requires
// dialect.Dialect.SupportsRowLocking.
func (q *Query[T]) ForShare() *Query[T] {
	nq := q.clone()
	nq.forShare = true
	return nq
}

// SkipLocked adds SKIP LOCKED to a ForUpdate/ForShare clause.
func (q *Query[T]) SkipLocked() *Query[T] {
	nq := q.clone()
	nq.skipLocked = true
	return nq
}

// SkipHooks lets a set-based Update or Delete/DeleteAll run even though T
// implements a hook relevant to that operation, which otherwise blocks it
// with ErrSetOperationBlockedByHooks (the hook simply won't fire, since no
// Go instance exists per affected row).
func (q *Query[T]) SkipHooks() *Query[T] {
	nq := q.clone()
	nq.skipHooks = true
	return nq
}

func (q *Query[T]) selectColumns() []sqlbuild.Column {
	if len(q.selectCols) > 0 {
		cols := make([]sqlbuild.Column, len(q.selectCols))
		for i, c := range q.selectCols {
			cols[i] = c.sqlColumn()

		}

		return cols
	}

	cols := make([]sqlbuild.Column, len(q.table.Fields))
	for i, f := range q.table.Fields {
		cols[i] = sqlbuild.QualifiedCol(q.table.Alias, f.Column)
	}

	return cols
}

func (q *Query[T]) groupByColumns() []sqlbuild.Column {
	cols := make([]sqlbuild.Column, len(q.groupBy))
	for i, c := range q.groupBy {
		cols[i] = c.sqlColumn()
	}

	return cols
}

// buildSelect assembles every clause accumulated on q into a
// sqlbuild.SelectBuilder, ready to Render.
func (q *Query[T]) buildSelect() *sqlbuild.SelectBuilder {
	b := sqlbuild.Select(q.sqlTable).Columns(q.selectColumns()...)

	if q.distinct {
		b = b.Distinct()
	}

	for _, raw := range q.selectRaw {
		b = b.SelectRaw(raw)
	}

	for _, j := range q.joins {
		if j.left {
			b = b.LeftJoin(j.table, j.on)
		} else {
			b = b.Join(j.table, j.on)
		}
	}

	if !q.where.IsZero() {
		b = b.Where(q.where)
	}

	if len(q.groupBy) > 0 {
		b = b.GroupBy(q.groupByColumns()...)
	}

	if !q.having.IsZero() {
		b = b.Having(q.having)
	}

	if len(q.order) > 0 {
		b = b.OrderBy(q.order...)
	}

	if q.limit != nil {
		b = b.Limit(*q.limit)
	}

	if q.offset != nil {
		b = b.Offset(*q.offset)
	}

	if q.forUpdate {
		b = b.ForUpdate()
	}

	if q.forShare {
		b = b.ForShare()
	}

	if q.skipLocked {
		b = b.SkipLocked()
	}

	return b
}

// Find runs q and returns every matching row, running any Preload chains
// afterward.
func (q *Query[T]) Find(ctx context.Context) ([]T, error) {
	sqlText, args, err := q.buildSelect().Render(q.db.dialect)
	if err != nil {
		return nil, err
	}

	rows, err := q.db.queryContext(ctx, "Find", q.table.GoType.Name(), sqlText, args)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result, err := scanAllRows[T](ctx, rows, q.table)
	if err != nil {
		return nil, err
	}

	for _, run := range q.preloads {
		if err := run(ctx, q.db, result); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// Rows runs q and streams matching rows one at a time instead of loading
// them all into memory, stopping early if the yield func returns false or
// ctx is done. It cannot be combined with Preload — batch-loading a
// relation needs every parent row up front, which streaming doesn't
// provide — and returns an error instead of iterating if any Preload was
// set.
func (q *Query[T]) Rows(ctx context.Context) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		var zero T
		if len(q.preloads) > 0 {
			yield(zero, fmt.Errorf("pack: Query[%s].Rows cannot be combined with Preload", q.table.GoType.Name()))
			return
		}

		sqlText, args, err := q.buildSelect().Render(q.db.dialect)
		if err != nil {
			yield(zero, err)
			return
		}

		rows, err := q.db.queryContext(ctx, "Rows", q.table.GoType.Name(), sqlText, args)
		if err != nil {
			yield(zero, err)
			return
		}
		defer func() { _ = rows.Close() }()

		cols, err := rows.Columns()
		if err != nil {
			yield(zero, err)
			return
		}

		plan := scan.NewPlan(q.table, cols)

		for {
			if err := ctx.Err(); err != nil {
				yield(zero, err)
				return
			}

			row, err := scanOneWithPlan[T](ctx, rows, plan, q.table)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return
				}
				yield(zero, err)
				return
			}
			if !yield(row, nil) {
				return
			}
		}
	}

}

// First runs q with an added LIMIT 1 and returns the single matching row,
// running any Preload chains on it afterward. Unlike Find, it returns
// ErrNoRows if nothing matches.
func (q *Query[T]) First(ctx context.Context) (T, error) {
	var zero T

	b := q.buildSelect().Limit(1)
	sqlText, args, err := b.Render(q.db.dialect)
	if err != nil {
		return zero, err
	}

	rows, err := q.db.queryContext(ctx, "First", q.table.GoType.Name(), sqlText, args)
	if err != nil {
		return zero, err
	}
	defer func() { _ = rows.Close() }()

	row, err := scanOneRow[T](ctx, rows, q.table)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return zero, ErrNoRows
		}
		return zero, err
	}

	if len(q.preloads) > 0 {
		wrapped := []T{row}
		for _, run := range q.preloads {
			if err := run(ctx, q.db, wrapped); err != nil {
				return zero, err
			}
		}

		row = wrapped[0]
	}

	return row, nil
}

// Count returns the number of rows q's WHERE/GROUP BY/HAVING clauses
// match.
func (q *Query[T]) Count(ctx context.Context) (int64, error) {
	b := sqlbuild.Select(q.sqlTable).SelectRaw("count(*)")
	if !q.where.IsZero() {
		b = b.Where(q.where)
	}
	if len(q.groupBy) > 0 {
		b = b.GroupBy(q.groupByColumns()...)
	}
	if !q.having.IsZero() {
		b = b.Having(q.having)
	}

	sqlText, args, err := b.Render(q.db.dialect)
	if err != nil {
		return 0, err
	}
	rows, err := q.db.queryContext(ctx, "Count", q.table.GoType.Name(), sqlText, args)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, err
		}
		return 0, nil
	}

	var n int64
	if err := rows.Scan(&n); err != nil {
		return 0, err
	}

	return n, nil
}

// Exists reports whether q's WHERE clause matches at least one row.
func (q *Query[T]) Exists(ctx context.Context) (bool, error) {
	b := sqlbuild.Select(q.sqlTable).SelectRaw("1").Limit(1)
	if !q.where.IsZero() {
		b = b.Where(q.where)
	}

	sqlText, args, err := b.Render(q.db.dialect)
	if err != nil {
		return false, err
	}
	rows, err := q.db.queryContext(ctx, "Exists", q.table.GoType.Name(), sqlText, args)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()

	has := rows.Next()
	if err := rows.Err(); err != nil {
		return false, err
	}

	return has, nil
}

// Update applies assignments to every row matching q's WHERE clause (every
// row of the table if none is set) and returns the number of rows
// affected. It's a set-based operation — it never materializes a Go T for
// each affected row — so it returns ErrSetOperationBlockedByHooks if T
// implements a before/after-update hook, unless SkipHooks was called.
func (q *Query[T]) Update(ctx context.Context, assignments ...Assignment) (int64, error) {
	if !q.skipHooks && (q.table.Hooks.BeforeUpdate || q.table.Hooks.AfterUpdate) {
		return 0, &ErrSetOperationBlockedByHooks{Model: q.table.GoType.Name(), Operation: "Update"}
	}

	raw := make([]sqlbuild.Assignment, len(assignments))
	for i, a := range assignments {
		raw[i] = a.a
	}

	b := sqlbuild.Update(q.sqlTable).Set(raw...)
	if !q.where.IsZero() {
		b = b.Where(q.where)
	}

	sqlText, args, err := b.Render(q.db.dialect)
	if err != nil {
		return 0, err
	}

	res, err := q.db.execContext(ctx, "Update", q.table.GoType.Name(), sqlText, args)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Delete deletes every row matching q's WHERE clause and returns the
// number of rows affected. As a safety rail against an accidental
// whole-table delete, it errors if no WHERE condition was set — call
// DeleteAll to delete every row deliberately. Subject to the same
// hook-blocking as Update.
func (q *Query[T]) Delete(ctx context.Context) (int64, error) {
	if q.where.IsZero() {
		return 0, fmt.Errorf(
			"pack: Query[%s].Delete: refusing to delete with no WHERE condition; call DeleteAll to delete every row",
			q.table.GoType.Name(),
		)
	}

	return q.deleteRows(ctx)
}

// DeleteAll deletes every row matching q's WHERE clause (every row of the
// table if none is set) and returns the number of rows affected. Unlike
// Delete, it does not require a WHERE condition. Subject to the same
// hook-blocking as Update.
func (q *Query[T]) DeleteAll(ctx context.Context) (int64, error) {
	return q.deleteRows(ctx)
}

func (q *Query[T]) deleteRows(ctx context.Context) (int64, error) {
	if !q.skipHooks && (q.table.Hooks.BeforeDelete || q.table.Hooks.AfterDelete) {
		return 0, &ErrSetOperationBlockedByHooks{Model: q.table.GoType.Name(), Operation: "Delete"}
	}

	b := sqlbuild.Delete(q.sqlTable)
	if !q.where.IsZero() {
		b = b.Where(q.where)
	}

	sqlText, args, err := b.Render(q.db.dialect)
	if err != nil {
		return 0, err
	}
	res, err := q.db.execContext(ctx, "Delete", q.table.GoType.Name(), sqlText, args)
	if err != nil {
		return 0, err
	}

	return res.RowsAffected()
}

// schemaForOrPanic resolves T's schema.Table or panics naming caller —
// used by every entry point (Of, Create, Relation, Join, ...) that takes a
// pack model as a type parameter instead of a value.
func schemaForOrPanic[T any](caller string) *schema.Table {
	t := reflect.TypeFor[T]()
	table, err := schema.For(t)
	if err != nil {
		panic(fmt.Sprintf("pack: %s[%s]: %v", caller, t.Name(), err))
	}

	return table
}

// appendFresh returns a fresh slice combining base and more, so a chain
// method's clone doesn't share (and risk mutating) another Query's
// backing array.
func appendFresh[E any](base []E, more ...E) []E {
	out := make([]E, len(base)+len(more))
	copy(out, base)
	copy(out[len(base):], more)
	return out
}

// andJoin AND-s b onto a, or just returns b if a hasn't been set yet.
func andJoin(a, b sqlbuild.Predicate) sqlbuild.Predicate {
	if a.IsZero() {
		return b
	}

	return sqlbuild.And(a, b)
}

// orJoin OR-s b onto a, or just returns b if a hasn't been set yet.
func orJoin(a, b sqlbuild.Predicate) sqlbuild.Predicate {
	if a.IsZero() {
		return b
	}

	return sqlbuild.Or(a, b)
}
