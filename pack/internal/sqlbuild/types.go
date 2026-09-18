package sqlbuild

// Table names a table (or, with Alias set, a table given a shorter alias
// for the query it appears in), e.g. for use in Select or a Join.
type Table struct {
	Name  string
	Alias string
}

// Column references one column, optionally qualified by a table/alias name
// (see QualifiedCol) to disambiguate it in a join. Build one with Col or
// QualifiedCol.
type Column struct {
	Table string
	Name  string
}

// Col references an unqualified column, e.g. for a query with no join.
func Col(name string) Column { return Column{Name: name} }

// QualifiedCol references a column qualified by tableAlias, e.g. to
// disambiguate it across the tables/aliases of a joined query.
func QualifiedCol(tableAlias, name string) Column { return Column{Table: tableAlias, Name: name} }

// Assignment is one "column = value" pair for an Insert or Update, built
// with Set or SetExpr.
type Assignment struct {
	Col     Column
	isRaw   bool
	val     any
	rawSQL  string
	rawArgs []any
}

// Set assigns c the literal value v, bound as a placeholder argument.
func Set(c Column, v any) Assignment {
	return Assignment{Col: c, val: v}
}

// SetExpr assigns c the raw SQL expression sqlExpr (e.g. "count = count +
// 1"), with args bound as its own placeholder arguments.
func SetExpr(c Column, sqlExpr string, args ...any) Assignment {
	return Assignment{Col: c, isRaw: true, rawSQL: sqlExpr, rawArgs: args}
}

// OrderTerm is one ORDER BY term, built with Asc, Desc, or OrderRaw.
type OrderTerm struct {
	col  Column
	desc bool
	raw  string
}

// Asc orders by c ascending.
func Asc(c Column) OrderTerm { return OrderTerm{col: c} }

// Desc orders by c descending.
func Desc(c Column) OrderTerm { return OrderTerm{col: c, desc: true} }

// OrderRaw orders by a raw SQL fragment (e.g. a function call) instead of a
// plain column.
func OrderRaw(fragment string) OrderTerm { return OrderTerm{raw: fragment} }

// JoinKind is the kind of SQL join a Join renders.
type JoinKind int

const (
	// InnerJoin renders as JOIN.
	InnerJoin JoinKind = iota
	// LeftJoin renders as LEFT JOIN.
	LeftJoin
)

// Join is one join clause of a SelectBuilder, added via
// SelectBuilder.Join/LeftJoin.
type Join struct {
	kind  JoinKind
	table Table
	on    Predicate
}
