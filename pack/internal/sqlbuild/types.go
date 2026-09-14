package sqlbuild

type Table struct {
	Name  string
	Alias string
}

type Column struct {
	Table string
	Name  string
}

func Col(name string) Column { return Column{Name: name} }

func QualifiedCol(tableAlias, name string) Column { return Column{Table: tableAlias, Name: name} }

type Assignment struct {
	Col     Column
	isRaw   bool
	val     any
	rawSQL  string
	rawArgs []any
}

func Set(c Column, v any) Assignment {
	return Assignment{Col: c, val: v}
}

func SetExpr(c Column, sqlExpr string, args ...any) Assignment {
	return Assignment{Col: c, isRaw: true, rawSQL: sqlExpr, rawArgs: args}
}

type OrderTerm struct {
	col  Column
	desc bool
	raw  string
}

func Asc(c Column) OrderTerm { return OrderTerm{col: c} }

func Desc(c Column) OrderTerm { return OrderTerm{col: c, desc: true} }

func OrderRaw(fragment string) OrderTerm { return OrderTerm{raw: fragment} }

type JoinKind int

const (
	InnerJoin JoinKind = iota
	LeftJoin
)

type Join struct {
	kind  JoinKind
	table Table
	on    Predicate
}
