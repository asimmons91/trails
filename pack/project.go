package pack

// Project re-targets q's underlying SQL table, joins, and filters at a
// different result type U, dropping T's own preloads/select
// list/skip-hooks state. Since the returned Query[U] defaults to U's
// mapped columns (see selectColumns), this is how to scan the same
// query's rows into a narrower struct — e.g. a subset of T's columns, or
// columns pulled across a join — without redeclaring the WHERE/JOIN
// clauses. Panics if U isn't a valid pack model.
func (q *Query[T]) Project[U any]() *Query[U] {
	uTable := schemaForOrPanic[U]("Project")
	return &Query[U]{
		db:        q.db,
		table:     uTable,
		sqlTable:  q.sqlTable,
		joins:     q.joins,
		where:     q.where,
		having:    q.having,
		order:     q.order,
		limit:     q.limit,
		offset:    q.offset,
		distinct:  q.distinct,
		forUpdate: q.forUpdate,
		forShare:  q.forShare,
	}
}
