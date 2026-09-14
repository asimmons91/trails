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
	preloads   []preloadRunner
	skipHooks  bool
}

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

func (q *Query[T]) clone() *Query[T] {
	nq := *q
	return &nq
}

func (q *Query[T]) Where(p Predicate) *Query[T] {
	nq := q.clone()
	nq.where = andJoin(nq.where, p.p)
	return nq
}

func (q *Query[T]) WhereRaw(fragment string, args ...any) *Query[T] {
	return q.Where(rawPredicate(fragment, args...))
}

func (q *Query[T]) Or(p Predicate) *Query[T] {
	nq := q.clone()
	nq.where = orJoin(nq.where, p.p)
	return nq
}

func (q *Query[T]) Order(terms ...OrderTerm[T]) *Query[T] {
	nq := q.clone()
	raw := make([]sqlbuild.OrderTerm, len(terms))
	for i, t := range terms {
		raw[i] = t.o
	}
	nq.order = appendFresh(nq.order, raw...)
	return nq
}

func (q *Query[T]) OrderRaw(fragment string) *Query[T] {
	nq := q.clone()
	nq.order = appendFresh(nq.order, sqlbuild.OrderRaw(fragment))
	return nq
}

func (q *Query[T]) Limit(n int64) *Query[T] {
	nq := q.clone()
	nq.limit = &n
	return nq
}

func (q *Query[T]) Offset(n int64) *Query[T] {
	nq := q.clone()
	nq.offset = &n
	return nq
}

func (q *Query[T]) GroupBy(cols ...AnyCol[T]) *Query[T] {
	nq := q.clone()
	nq.groupBy = appendFresh(nq.groupBy, cols...)
	return nq
}

func (q *Query[T]) Having(p Predicate) *Query[T] {
	nq := q.clone()
	nq.having = andJoin(nq.having, p.p)
	return nq
}

func (q *Query[T]) Distinct() *Query[T] {
	nq := q.clone()
	nq.distinct = true
	return nq
}

func (q *Query[T]) Select(cols ...AnyCol[T]) *Query[T] {
	nq := q.clone()
	nq.selectCols = appendFresh(nq.selectCols, cols...)
	return nq
}

func (q *Query[T]) SelectRaw(fragment string) *Query[T] {
	nq := q.clone()
	nq.selectRaw = appendFresh(nq.selectRaw, fragment)
	return nq
}

func (q *Query[T]) ForUpdate() *Query[T] {
	nq := q.clone()
	nq.forUpdate = true
	return nq
}

func (q *Query[T]) ForShare() *Query[T] {
	nq := q.clone()
	nq.forShare = true
	return nq
}

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

	return b
}

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

func (q *Query[T]) Delete(ctx context.Context) (int64, error) {
	if q.where.IsZero() {
		return 0, fmt.Errorf(
			"pack: Query[%s].Delete: refusing to delete with no WHERE condition; call DeleteAll to delete every row",
			q.table.GoType.Name(),
		)
	}

	return q.deleteRows(ctx)
}

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

func schemaForOrPanic[T any](caller string) *schema.Table {
	t := reflect.TypeFor[T]()
	table, err := schema.For(t)
	if err != nil {
		panic(fmt.Sprintf("pack: %s[%s]: %v", caller, t.Name(), err))
	}

	return table
}

func appendFresh[E any](base []E, more ...E) []E {
	out := make([]E, len(base)+len(more))
	copy(out, base)
	copy(out[len(base):], more)
	return out
}

func andJoin(a, b sqlbuild.Predicate) sqlbuild.Predicate {
	if a.IsZero() {
		return b
	}

	return sqlbuild.And(a, b)
}

func orJoin(a, b sqlbuild.Predicate) sqlbuild.Predicate {
	if a.IsZero() {
		return b
	}

	return sqlbuild.Or(a, b)
}
