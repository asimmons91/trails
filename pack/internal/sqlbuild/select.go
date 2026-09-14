package sqlbuild

import (
	"strconv"
	"strings"

	"github.com/asimmons91/trails/pack/dialect"
)

type SelectBuilder struct {
	table     Table
	joins     []Join
	distinct  bool
	columns   []Column
	selectRaw []string
	where     Predicate
	groupBy   []Column
	having    Predicate
	order     []OrderTerm
	limit     *int64
	offset    *int64
	forUpdate bool
	forShare  bool
}

func Select(t Table) *SelectBuilder {
	return &SelectBuilder{table: t}
}

func (b *SelectBuilder) clone() *SelectBuilder {
	nb := *b
	return &nb
}

func (b *SelectBuilder) Columns(cols ...Column) *SelectBuilder {
	nb := b.clone()
	nb.columns = appendFresh(nb.columns, cols...)
	return nb
}

func (b *SelectBuilder) SelectRaw(fragment string) *SelectBuilder {
	nb := b.clone()
	nb.selectRaw = appendFresh(nb.selectRaw, fragment)
	return nb
}

func (b *SelectBuilder) Distinct() *SelectBuilder {
	nb := b.clone()
	nb.distinct = true
	return nb
}

func (b *SelectBuilder) Join(t Table, on Predicate) *SelectBuilder {
	nb := b.clone()
	nb.joins = appendFresh(nb.joins, Join{kind: InnerJoin, table: t, on: on})
	return nb
}

func (b *SelectBuilder) LeftJoin(t Table, on Predicate) *SelectBuilder {
	nb := b.clone()
	nb.joins = appendFresh(nb.joins, Join{kind: LeftJoin, table: t, on: on})
	return nb
}

func (b *SelectBuilder) Where(p Predicate) *SelectBuilder {
	nb := b.clone()
	nb.where = andJoin(nb.where, p)
	return nb
}

func (b *SelectBuilder) GroupBy(cols ...Column) *SelectBuilder {
	nb := b.clone()
	nb.groupBy = appendFresh(nb.groupBy, cols...)
	return nb
}

func (b *SelectBuilder) Having(p Predicate) *SelectBuilder {
	nb := b.clone()
	nb.having = andJoin(nb.having, p)
	return nb
}

func (b *SelectBuilder) OrderBy(terms ...OrderTerm) *SelectBuilder {
	nb := b.clone()
	nb.order = appendFresh(nb.order, terms...)
	return nb
}

func (b *SelectBuilder) Limit(n int64) *SelectBuilder {
	nb := b.clone()
	nb.limit = &n
	return nb
}

func (b *SelectBuilder) Offset(n int64) *SelectBuilder {
	nb := b.clone()
	nb.offset = &n
	return nb
}

func (b *SelectBuilder) ForUpdate() *SelectBuilder {
	nb := b.clone()
	nb.forUpdate = true
	return nb
}

func (b *SelectBuilder) ForShare() *SelectBuilder {
	nb := b.clone()
	nb.forShare = true
	return nb
}

func (b *SelectBuilder) Render(d dialect.Dialect) (string, []any, error) {
	if b.forUpdate && b.forShare {
		return "", nil, &ErrConflictingLockClause{}
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

	return sb.String(), r.args, nil
}

func andJoin(a, b Predicate) Predicate {
	if a.IsZero() {
		return b
	}

	return And(a, b)
}
