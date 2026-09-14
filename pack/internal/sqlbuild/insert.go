package sqlbuild

import (
	"strings"

	"github.com/asimmons91/trails/pack/dialect"
)

type InsertBuilder struct {
	table     Table
	rows      [][]Assignment
	returning []Column

	conflictCols      []Column
	conflictDoNothing bool
	conflictUpdate    []Assignment
}

func Insert(t Table) *InsertBuilder {
	return &InsertBuilder{table: t}
}

func (b *InsertBuilder) clone() *InsertBuilder {
	nb := *b
	return &nb
}

func (b *InsertBuilder) Values(a ...Assignment) *InsertBuilder {
	nb := b.clone()
	nb.rows = appendFresh(nb.rows, append([]Assignment(nil), a...))
	return nb
}

func (b *InsertBuilder) Returning(cols ...Column) *InsertBuilder {
	nb := b.clone()
	nb.returning = appendFresh(nb.returning, cols...)
	return nb
}

func (b *InsertBuilder) OnConflictDoNothing(cols ...Column) *InsertBuilder {
	nb := b.clone()
	nb.conflictCols = append([]Column(nil), cols...)
	nb.conflictDoNothing = true
	nb.conflictUpdate = nil
	return nb
}

func (b *InsertBuilder) OnConflictDoUpdate(cols []Column, assignments ...Assignment) *InsertBuilder {
	nb := b.clone()
	nb.conflictCols = append([]Column(nil), cols...)
	nb.conflictDoNothing = false
	nb.conflictUpdate = append([]Assignment(nil), assignments...)
	return nb
}

func (b *InsertBuilder) Render(d dialect.Dialect) (string, []any, error) {
	r := newRenderer(d)
	var sb strings.Builder

	sb.WriteString("INSERT INTO ")
	sb.WriteString(r.d.QuoteIdent(b.table.Name))

	if len(b.rows) > 0 {
		cols := make([]string, len(b.rows[0]))
		for i, a := range b.rows[0] {
			cols[i] = r.d.QuoteIdent(a.Col.Name)
		}
		sb.WriteString(" (")
		sb.WriteString(strings.Join(cols, ", "))
		sb.WriteString(") VALUES ")

		rowStrs := make([]string, len(b.rows))
		for ri, row := range b.rows {
			vals := make([]string, len(row))

			for i, a := range row {
				if a.isRaw {
					expr, err := r.renderRawFragment(a.rawSQL, a.rawArgs)
					if err != nil {
						return "", nil, err
					}

					vals[i] = expr
				} else {
					vals[i] = r.bind(a.val)
				}
			}

			rowStrs[ri] = "(" + strings.Join(vals, ", ") + ")"
		}

		sb.WriteString(strings.Join(rowStrs, ", "))
	}

	if len(b.conflictCols) > 0 {
		sb.WriteString(" ON CONFLICT (")
		colNames := make([]string, len(b.conflictCols))

		for i, c := range b.conflictCols {
			colNames[i] = r.d.QuoteIdent(c.Name)
		}
		sb.WriteString(strings.Join(colNames, ", "))
		sb.WriteString(")")

		if b.conflictDoNothing {
			sb.WriteString(" DO NOTHING")
		} else {
			sb.WriteString(" DO UPDATE SET ")
			sets := make([]string, len(b.conflictUpdate))
			for i, a := range b.conflictUpdate {
				s, err := r.renderAssignment(a)
				if err != nil {
					return "", nil, err
				}
				sets[i] = s
			}
			sb.WriteString(strings.Join(sets, ", "))
		}
	}

	if len(b.returning) > 0 {
		retCols := make([]string, len(b.returning))
		for i, c := range b.returning {
			retCols[i] = r.quoteColumn(c)
		}
		sb.WriteString(" RETURNING ")
		sb.WriteString(strings.Join(retCols, ", "))
	}

	return sb.String(), r.args, nil
}
