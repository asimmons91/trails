package pack

import "github.com/asimmons91/trails/pack/internal/sqlbuild"

type queryJoin struct {
	left  bool
	table sqlbuild.Table
	on    sqlbuild.Predicate
}

func (q *Query[T]) Join[U any](on Predicate) *Query[T] {
	uTable := schemaForOrPanic[U]("Join")
	nq := q.clone()
	nq.joins = appendFresh(nq.joins, queryJoin{
		table: sqlbuild.Table{Name: uTable.Name, Alias: uTable.Alias},
		on:    on.p,
	})

	return nq
}

func (q *Query[T]) LeftJoin[U any](on Predicate) *Query[T] {
	uTable := schemaForOrPanic[U]("LeftJoin")
	nq := q.clone()
	nq.joins = appendFresh(nq.joins, queryJoin{
		left:  true,
		table: sqlbuild.Table{Name: uTable.Name, Alias: uTable.Alias},
		on:    on.p,
	})
	return nq
}

func On[L, R, F any](left Col[L, F], right Col[R, F]) Predicate {
	return Predicate{p: sqlbuild.EqCol(left.col, right.col)}
}
