// Package sqlbuild is a dialect-aware SQL builder: construct a
// *SelectBuilder, *InsertBuilder, *UpdateBuilder, *DeleteBuilder,
// *CreateTableBuilder, or one of the smaller DDL builders (AddColumn,
// CreateIndex, ...), chain its clause methods, then call
// Render(dialect.Dialect) to get back (sql string, args []any, error) for
// that dialect. pack/query.go drives SelectBuilder/UpdateBuilder/
// DeleteBuilder/InsertBuilder for ordinary queries, and pack/migrate drives
// the DDL builders for schema changes.
package sqlbuild

import (
	"strconv"
	"strings"

	"github.com/asimmons91/trails/pack/dialect"
)

// SelectBuilder builds a SELECT statement. Build one with Select; every
// clause method returns a new *SelectBuilder rather than mutating the
// receiver, so a builder can be branched and reused safely.
type SelectBuilder struct {
	table      Table
	joins      []Join
	distinct   bool
	columns    []Column
	selectRaw  []string
	where      Predicate
	groupBy    []Column
	having     Predicate
	order      []OrderTerm
	limit      *int64
	offset     *int64
	forUpdate  bool
	forShare   bool
	skipLocked bool
}

// Select starts a SelectBuilder reading from t.
func Select(t Table) *SelectBuilder {
	return &SelectBuilder{table: t}
}

func (b *SelectBuilder) clone() *SelectBuilder {
	nb := *b
	return &nb
}

// Columns adds cols to the SELECT list. With no Columns/SelectRaw calls at
// all, Render selects "*".
func (b *SelectBuilder) Columns(cols ...Column) *SelectBuilder {
	nb := b.clone()
	nb.columns = appendFresh(nb.columns, cols...)
	return nb
}

// SelectRaw adds a raw SQL fragment (e.g. an aggregate expression) to the
// SELECT list, alongside any Columns.
func (b *SelectBuilder) SelectRaw(fragment string) *SelectBuilder {
	nb := b.clone()
	nb.selectRaw = appendFresh(nb.selectRaw, fragment)
	return nb
}

// Distinct adds DISTINCT to the SELECT list.
func (b *SelectBuilder) Distinct() *SelectBuilder {
	nb := b.clone()
	nb.distinct = true
	return nb
}

// Join adds an inner JOIN against t with the given ON predicate.
func (b *SelectBuilder) Join(t Table, on Predicate) *SelectBuilder {
	nb := b.clone()
	nb.joins = appendFresh(nb.joins, Join{kind: InnerJoin, table: t, on: on})
	return nb
}

// LeftJoin adds a LEFT JOIN against t with the given ON predicate.
func (b *SelectBuilder) LeftJoin(t Table, on Predicate) *SelectBuilder {
	nb := b.clone()
	nb.joins = appendFresh(nb.joins, Join{kind: LeftJoin, table: t, on: on})
	return nb
}

// Where AND-combines p with any predicate already set by a prior Where
// call.
func (b *SelectBuilder) Where(p Predicate) *SelectBuilder {
	nb := b.clone()
	nb.where = andJoin(nb.where, p)
	return nb
}

// GroupBy adds cols to the GROUP BY clause.
func (b *SelectBuilder) GroupBy(cols ...Column) *SelectBuilder {
	nb := b.clone()
	nb.groupBy = appendFresh(nb.groupBy, cols...)
	return nb
}

// Having AND-combines p with any predicate already set by a prior Having
// call.
func (b *SelectBuilder) Having(p Predicate) *SelectBuilder {
	nb := b.clone()
	nb.having = andJoin(nb.having, p)
	return nb
}

// OrderBy adds terms to the ORDER BY clause.
func (b *SelectBuilder) OrderBy(terms ...OrderTerm) *SelectBuilder {
	nb := b.clone()
	nb.order = appendFresh(nb.order, terms...)
	return nb
}

// Limit sets the LIMIT clause to n.
func (b *SelectBuilder) Limit(n int64) *SelectBuilder {
	nb := b.clone()
	nb.limit = &n
	return nb
}

// Offset sets the OFFSET clause to n.
func (b *SelectBuilder) Offset(n int64) *SelectBuilder {
	nb := b.clone()
	nb.offset = &n
	return nb
}

// ForUpdate adds a FOR UPDATE row-locking clause. Render errors if the
// dialect doesn't support row locking, or if ForShare was also called.
func (b *SelectBuilder) ForUpdate() *SelectBuilder {
	nb := b.clone()
	nb.forUpdate = true
	return nb
}

// ForShare adds a FOR SHARE row-locking clause. Render errors if the
// dialect doesn't support row locking, or if ForUpdate was also called.
func (b *SelectBuilder) ForShare() *SelectBuilder {
	nb := b.clone()
	nb.forShare = true
	return nb
}

// SkipLocked adds SKIP LOCKED. Render errors unless ForUpdate or ForShare
// was also called.
func (b *SelectBuilder) SkipLocked() *SelectBuilder {
	nb := b.clone()
	nb.skipLocked = true
	return nb
}

// Render renders b as SELECT SQL for dialect d, returning the statement,
// its bind arguments in placeholder order, and an error if b combines
// clauses d can't express (conflicting/unsupported locking, an
// out-of-range raw placeholder, ...).
func (b *SelectBuilder) Render(d dialect.Dialect) (string, []any, error) {
	if b.forUpdate && b.forShare {
		return "", nil, &ErrConflictingLockClause{}
	}

	if b.skipLocked && !b.forUpdate && !b.forShare {
		return "", nil, &ErrSkipLockedRequiresLockClause{}
	}

	if (b.forUpdate || b.forShare) && !d.SupportsRowLocking() {
		return "", nil, &ErrRowLockingUnsupportedByDialect{Dialect: d.Name()}
	}

	r := newRenderer(d)
	var sb strings.Builder

	sb.WriteString("SELECT ")
	if b.distinct {
		sb.WriteString("DISTINCT ")
	}

	selectList := make([]string, 0, len(b.columns)+len(b.selectRaw))

	for _, c := range b.columns {
		selectList = append(selectList, r.quoteColumn(c))
	}
	selectList = append(selectList, b.selectRaw...)
	if len(selectList) == 0 {
		sb.WriteString("*")
	} else {
		sb.WriteString(strings.Join(selectList, ", "))
	}

	sb.WriteString(" FROM ")
	sb.WriteString(r.quoteTable(b.table))

	for _, j := range b.joins {
		joinSQL, err := r.renderJoin(j)
		if err != nil {
			return "", nil, err
		}
		sb.WriteString(joinSQL)
	}

	if !b.where.IsZero() {
		whereSQL, err := r.renderPredicate(b.where)
		if err != nil {
			return "", nil, err
		}
		sb.WriteString(" WHERE ")
		sb.WriteString(whereSQL)
	}

	if len(b.groupBy) > 0 {
		cols := make([]string, len(b.groupBy))
		for i, c := range b.groupBy {
			cols[i] = r.quoteColumn(c)
		}
		sb.WriteString(" GROUP BY ")
		sb.WriteString(strings.Join(cols, ", "))
	}

	if !b.having.IsZero() {
		havingSQL, err := r.renderPredicate(b.having)
		if err != nil {
			return "", nil, err
		}
		sb.WriteString(" HAVING ")
		sb.WriteString(havingSQL)
	}

	if len(b.order) > 0 {
		terms := make([]string, len(b.order))
		for i, o := range b.order {
			terms[i] = r.renderOrderTerm(o)
		}
		sb.WriteString(" ORDER BY ")
		sb.WriteString(strings.Join(terms, ", "))
	}

	if b.limit != nil {
		sb.WriteString(" LIMIT ")
		sb.WriteString(strconv.FormatInt(*b.limit, 10))
	}

	if b.offset != nil {
		sb.WriteString(" OFFSET ")
		sb.WriteString(strconv.FormatInt(*b.offset, 10))
	}

	if b.forUpdate {
		sb.WriteString(" FOR UPDATE")
	} else if b.forShare {
		sb.WriteString(" FOR SHARE")
	}

	if b.skipLocked {
		sb.WriteString(" SKIP LOCKED")
	}

	return sb.String(), r.args, nil
}

// andJoin AND-combines a and b, or simply returns b if a is unset (see
// Predicate.IsZero) — used by every builder's Where/Having so the first
// call doesn't wrap a lone predicate in an unnecessary And.
func andJoin(a, b Predicate) Predicate {
	if a.IsZero() {
		return b
	}

	return And(a, b)
}
