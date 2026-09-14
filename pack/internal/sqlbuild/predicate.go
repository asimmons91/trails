package sqlbuild

type predKind int

const (
	predNone predKind = iota
	predEq
	predNe
	predGt
	predGte
	predLt
	predLte
	predIn
	predNotIn
	predIsNull
	predIsNotNull
	predBetween
	predLike
	predILike
	predNotLike
	predAnd
	predOr
	predNot
	predRaw
	predEqCol
)

type Predicate struct {
	kind     predKind
	col      Column
	rightCol Column
	args     []any
	children []Predicate
	rawSQL   string
}

func (p Predicate) IsZero() bool { return p.kind == predNone }

func Eq(c Column, v any) Predicate  { return Predicate{kind: predEq, col: c, args: []any{v}} }
func Ne(c Column, v any) Predicate  { return Predicate{kind: predNe, col: c, args: []any{v}} }
func Gt(c Column, v any) Predicate  { return Predicate{kind: predGt, col: c, args: []any{v}} }
func Gte(c Column, v any) Predicate { return Predicate{kind: predGte, col: c, args: []any{v}} }
func Lt(c Column, v any) Predicate  { return Predicate{kind: predLt, col: c, args: []any{v}} }
func Lte(c Column, v any) Predicate { return Predicate{kind: predLte, col: c, args: []any{v}} }

func In(c Column, vs ...any) Predicate {
	return Predicate{kind: predIn, col: c, args: append([]any(nil), vs...)}
}

func NotIn(c Column, vs ...any) Predicate {
	return Predicate{kind: predNotIn, col: c, args: append([]any(nil), vs...)}
}

func IsNull(c Column) Predicate    { return Predicate{kind: predIsNull, col: c} }
func IsNotNull(c Column) Predicate { return Predicate{kind: predIsNotNull, col: c} }

func Between(c Column, lo, hi any) Predicate {
	return Predicate{kind: predBetween, col: c, args: []any{lo, hi}}
}

func Like(c Column, pattern string) Predicate {
	return Predicate{kind: predLike, col: c, args: []any{pattern}}
}

func ILike(c Column, pattern string) Predicate {
	return Predicate{kind: predILike, col: c, args: []any{pattern}}
}

func NotLike(c Column, pattern string) Predicate {
	return Predicate{kind: predNotLike, col: c, args: []any{pattern}}
}

func Raw(fragment string, args ...any) Predicate {
	return Predicate{kind: predRaw, rawSQL: fragment, args: append([]any(nil), args...)}
}

func And(preds ...Predicate) Predicate {
	return Predicate{kind: predAnd, children: append([]Predicate(nil), preds...)}
}

func Or(preds ...Predicate) Predicate {
	return Predicate{kind: predOr, children: append([]Predicate(nil), preds...)}
}

func Not(p Predicate) Predicate {
	return Predicate{kind: predNot, children: []Predicate{p}}
}

func EqCol(left, right Column) Predicate {
	return Predicate{kind: predEqCol, col: left, rightCol: right}
}
