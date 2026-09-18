package sqlbuild

import "github.com/asimmons91/trails/pack/dialect"

// DeleteBuilder builds a DELETE statement. Build one with Delete. A
// DeleteBuilder with no Where call deletes every row when rendered.
type DeleteBuilder struct {
	table Table
	where Predicate
}

// Delete starts a DeleteBuilder deleting from t.
func Delete(t Table) *DeleteBuilder {
	return &DeleteBuilder{table: t}
}

func (b *DeleteBuilder) clone() *DeleteBuilder {
	nb := *b
	return &nb
}

// Where AND-combines p with any predicate already set by a prior Where
// call.
func (b *DeleteBuilder) Where(p Predicate) *DeleteBuilder {
	nb := b.clone()
	nb.where = andJoin(nb.where, p)
	return nb
}

// Render renders b as DELETE SQL for dialect d.
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
