package sqlbuild

import "github.com/asimmons91/trails/pack/dialect"

type DeleteBuilder struct {
	table Table
	where Predicate
}

func Delete(t Table) *DeleteBuilder {
	return &DeleteBuilder{table: t}
}

func (b *DeleteBuilder) clone() *DeleteBuilder {
	nb := *b
	return &nb
}

func (b *DeleteBuilder) Where(p Predicate) *DeleteBuilder {
	nb := b.clone()
	nb.where = andJoin(nb.where, p)
	return nb
}

func (b *DeleteBuilder) Render(d dialect.Dialect) (string, []any, error) {
	r := newRenderer(d)
	sql := "DELETE FROM " + r.quoteTable(b.table)

	if !b.where.IsZero() {
		whereSQL, err := r.renderPredicate(b.where)
		if err != nil {
			return "", nil, err
		}
		sql += " WHERE " + whereSQL
	}

	return sql, r.args, nil
}
