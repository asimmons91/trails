package pack

import "github.com/asimmons91/trails/pack/internal/sqlbuild"

// Predicate is a composable, type-erased boolean SQL condition — what
// Where, Having, and a Join's on clause all take. Build one from a Col's
// comparison methods (Eq, In, IsNull, ...) or combine existing ones with
// And, Or, and Not.
type Predicate struct {
	p sqlbuild.Predicate
}

// And combines preds with SQL AND.
func And(preds ...Predicate) Predicate {
	return Predicate{p: sqlbuild.And(unwrapPredicate(preds)...)}
}

// Or combines preds with SQL OR. This is distinct from Query[T].Or, which
// adds a top-level alternative clause to a query rather than combining
// Predicate values.
func Or(preds ...Predicate) Predicate {
	return Predicate{p: sqlbuild.Or(unwrapPredicate(preds)...)}
}

// Not negates p.
func Not(p Predicate) Predicate {
	return Predicate{p: sqlbuild.Not(p.p)}
}

func rawPredicate(fragment string, args ...any) Predicate {
	return Predicate{p: sqlbuild.Raw(fragment, args...)}
}

func unwrapPredicate(preds []Predicate) []sqlbuild.Predicate {
	out := make([]sqlbuild.Predicate, len(preds))
	for i, p := range preds {
		out[i] = p.p
	}

	return out
}

// Like builds a "col LIKE pattern" predicate.
func Like[T any](c Col[T, string], pattern string) Predicate {
	return Predicate{p: sqlbuild.Like(c.col, pattern)}
}

// ILike builds a case-insensitive "col ILIKE pattern" predicate. Requires
// dialect.Dialect.SupportsILike (Postgres does; other dialects error at
// render time).
func ILike[T any](c Col[T, string], pattern string) Predicate {
	return Predicate{p: sqlbuild.ILike(c.col, pattern)}
}

// NotLike builds a "col NOT LIKE pattern" predicate.
func NotLike[T any](c Col[T, string], pattern string) Predicate {
	return Predicate{p: sqlbuild.NotLike(c.col, pattern)}
}
