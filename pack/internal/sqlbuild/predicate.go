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

// Predicate is a composable WHERE/HAVING/ON condition, built via Eq, And,
// Or, and the other constructors in this file. The zero Predicate (see
// IsZero) means "no condition."
type Predicate struct {
	kind     predKind
	col      Column
	rightCol Column
	args     []any
	children []Predicate
	rawSQL   string
}

// IsZero reports whether p is the zero Predicate — "no condition" — as
// opposed to one built by a constructor like Eq.
func (p Predicate) IsZero() bool { return p.kind == predNone }

// Eq builds a "c = v" predicate.
func Eq(c Column, v any) Predicate { return Predicate{kind: predEq, col: c, args: []any{v}} }

// Ne builds a "c <> v" predicate.
func Ne(c Column, v any) Predicate { return Predicate{kind: predNe, col: c, args: []any{v}} }

// Gt builds a "c > v" predicate.
func Gt(c Column, v any) Predicate { return Predicate{kind: predGt, col: c, args: []any{v}} }

// Gte builds a "c >= v" predicate.
func Gte(c Column, v any) Predicate { return Predicate{kind: predGte, col: c, args: []any{v}} }

// Lt builds a "c < v" predicate.
func Lt(c Column, v any) Predicate { return Predicate{kind: predLt, col: c, args: []any{v}} }

// Lte builds a "c <= v" predicate.
func Lte(c Column, v any) Predicate { return Predicate{kind: predLte, col: c, args: []any{v}} }

// In builds a "c IN (vs...)" predicate. Render produces "FALSE" if vs is
// empty.
func In(c Column, vs ...any) Predicate {
	return Predicate{kind: predIn, col: c, args: append([]any(nil), vs...)}
}

// NotIn builds a "c NOT IN (vs...)" predicate. Render produces "TRUE" if
// vs is empty.
func NotIn(c Column, vs ...any) Predicate {
	return Predicate{kind: predNotIn, col: c, args: append([]any(nil), vs...)}
}

// IsNull builds a "c IS NULL" predicate.
func IsNull(c Column) Predicate { return Predicate{kind: predIsNull, col: c} }

// IsNotNull builds a "c IS NOT NULL" predicate.
func IsNotNull(c Column) Predicate { return Predicate{kind: predIsNotNull, col: c} }

// Between builds a "c BETWEEN lo AND hi" predicate.
func Between(c Column, lo, hi any) Predicate {
	return Predicate{kind: predBetween, col: c, args: []any{lo, hi}}
}

// Like builds a "c LIKE pattern" predicate.
func Like(c Column, pattern string) Predicate {
	return Predicate{kind: predLike, col: c, args: []any{pattern}}
}

// ILike builds a "c ILIKE pattern" predicate. Rendering it errors on a
// dialect without native case-insensitive LIKE (Dialect.SupportsILike).
func ILike(c Column, pattern string) Predicate {
	return Predicate{kind: predILike, col: c, args: []any{pattern}}
}

// NotLike builds a "c NOT LIKE pattern" predicate.
func NotLike(c Column, pattern string) Predicate {
	return Predicate{kind: predNotLike, col: c, args: []any{pattern}}
}

// Raw builds a predicate from a raw SQL fragment, with "$1", "$2", ...
// placeholders in fragment bound to args (renumbered into the dialect's
// own placeholder syntax and position when rendered).
func Raw(fragment string, args ...any) Predicate {
	return Predicate{kind: predRaw, rawSQL: fragment, args: append([]any(nil), args...)}
}

// And builds a predicate that's true only if every one of preds is,
// parenthesized as a unit when nested inside another And/Or/Not. Render
// produces "TRUE" if preds is empty.
func And(preds ...Predicate) Predicate {
	return Predicate{kind: predAnd, children: append([]Predicate(nil), preds...)}
}

// Or builds a predicate that's true if any one of preds is, parenthesized
// as a unit when nested inside another And/Or/Not. Render produces
// "FALSE" if preds is empty.
func Or(preds ...Predicate) Predicate {
	return Predicate{kind: predOr, children: append([]Predicate(nil), preds...)}
}

// Not negates p.
func Not(p Predicate) Predicate {
	return Predicate{kind: predNot, children: []Predicate{p}}
}

// EqCol builds a "left = right" predicate comparing two columns directly,
// rather than a column against a bound value — e.g. for a join's ON
// clause.
func EqCol(left, right Column) Predicate {
	return Predicate{kind: predEqCol, col: left, rightCol: right}
}
