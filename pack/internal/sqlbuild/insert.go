package sqlbuild

import (
	"strings"

	"github.com/asimmons91/trails/pack/dialect"
)

// InsertBuilder builds an INSERT statement, one or more rows at a time,
// with optional RETURNING and ON CONFLICT clauses. Build one with Insert.
type InsertBuilder struct {
	table     Table
	rows      [][]Assignment
	returning []Column

	conflictCols      []Column
	conflictDoNothing bool
	conflictUpdate    []Assignment
}

// Insert starts an InsertBuilder inserting into t.
func Insert(t Table) *InsertBuilder {
	return &InsertBuilder{table: t}
}

func (b *InsertBuilder) clone() *InsertBuilder {
	nb := *b
	return &nb
}

// Values adds one row of column assignments. Called more than once, each
// call adds another row to a single multi-row INSERT; every row must
// assign the same set of columns.
func (b *InsertBuilder) Values(a ...Assignment) *InsertBuilder {
	nb := b.clone()
	nb.rows = appendFresh(nb.rows, append([]Assignment(nil), a...))
	return nb
}

// Returning adds a RETURNING clause for cols. Render errors if the dialect
// doesn't support RETURNING (Dialect.SupportsReturning).
func (b *InsertBuilder) Returning(cols ...Column) *InsertBuilder {
	nb := b.clone()
	nb.returning = appendFresh(nb.returning, cols...)
	return nb
}

// OnConflictDoNothing makes a conflict on cols a no-op instead of an
// error. Render emits ON CONFLICT (cols) DO NOTHING on a dialect that
// supports it (Dialect.SupportsOnConflict), or MySQL's equivalent
// INSERT IGNORE INTO otherwise.
func (b *InsertBuilder) OnConflictDoNothing(cols ...Column) *InsertBuilder {
	nb := b.clone()
	nb.conflictCols = append([]Column(nil), cols...)
	nb.conflictDoNothing = true
	nb.conflictUpdate = nil
	return nb
}

// OnConflictDoUpdate makes a conflict on cols apply assignments to the
// existing row instead of erroring. Render emits
// ON CONFLICT (cols) DO UPDATE SET ... on a dialect that supports it
// (Dialect.SupportsOnConflict), or MySQL's equivalent
// ON DUPLICATE KEY UPDATE ... otherwise.
func (b *InsertBuilder) OnConflictDoUpdate(cols []Column, assignments ...Assignment) *InsertBuilder {
	nb := b.clone()
	nb.conflictCols = append([]Column(nil), cols...)
	nb.conflictDoNothing = false
	nb.conflictUpdate = append([]Assignment(nil), assignments...)
	return nb
}

// Render renders b as INSERT SQL for dialect d, choosing between
// Postgres/SQLite-style ON CONFLICT and MySQL-style
// INSERT IGNORE/ON DUPLICATE KEY UPDATE per Dialect.SupportsOnConflict.
// It errors with ErrReturningUnsupportedByDialect if Returning was called
// against a dialect that can't honor it.
func (b *InsertBuilder) Render(d dialect.Dialect) (string, []any, error) {
	if len(b.returning) > 0 && !d.SupportsReturning() {
		return "", nil, &ErrReturningUnsupportedByDialect{Dialect: d.Name()}
	}

	mysqlStyleConflict := len(b.conflictCols) > 0 && !d.SupportsOnConflict()

	r := newRenderer(d)
	var sb strings.Builder

	if mysqlStyleConflict && b.conflictDoNothing {
		sb.WriteString("INSERT IGNORE INTO ")
	} else {
		sb.WriteString("INSERT INTO ")
	}
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

	if len(b.conflictCols) > 0 && !mysqlStyleConflict {
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
	} else if mysqlStyleConflict && !b.conflictDoNothing {
		sb.WriteString(" ON DUPLICATE KEY UPDATE ")
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
