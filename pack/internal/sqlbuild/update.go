package sqlbuild

import (
	"strings"

	"github.com/asimmons91/trails/pack/dialect"
)

type UpdateBuilder struct {
	table       Table
	assignments []Assignment
	where       Predicate
}

func Update(t Table) *UpdateBuilder {
	return &UpdateBuilder{table: t}
}

func (b *UpdateBuilder) clone() *UpdateBuilder {
	nb := *b
	return &nb
}

func (b *UpdateBuilder) Set(a ...Assignment) *UpdateBuilder {
	nb := b.clone()
	nb.assignments = appendFresh(nb.assignments, a...)
	return nb
}

func (b *UpdateBuilder) Where(p Predicate) *UpdateBuilder {
	nb := b.clone()
	nb.where = andJoin(nb.where, p)
	return nb
}

func (b *UpdateBuilder) Render(d dialect.Dialect) (string, []any, error) {
	r := newRenderer(d)
	var sb strings.Builder

	sb.WriteString("UPDATE ")
	sb.WriteString(r.quoteTable(b.table))
	sb.WriteString(" SET ")

	sets := make([]string, len(b.assignments))
	for i, a := range b.assignments {
		s, err := r.renderAssignment(a)
		if err != nil {
			return "", nil, err
		}
		sets[i] = s
	}
	sb.WriteString(strings.Join(sets, ", "))

	if !b.where.IsZero() {
		whereSQL, err := r.renderPredicate(b.where)
		if err != nil {
			return "", nil, err
		}
		sb.WriteString(" WHERE ")
		sb.WriteString(whereSQL)
	}

	return sb.String(), r.args, nil
}
