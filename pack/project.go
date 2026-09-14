package pack

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
