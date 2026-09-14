package pack

import "github.com/asimmons91/trails/pack/internal/sqlbuild"

type Predicate struct {
	p sqlbuild.Predicate
}

func And(preds ...Predicate) Predicate {
	return Predicate{p: sqlbuild.And(unwrapPredicate(preds)...)}
}

func Or(preds ...Predicate) Predicate {
	return Predicate{p: sqlbuild.Or(unwrapPredicate(preds)...)}
}

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

func Like[T any](c Col[T, string], pattern string) Predicate {
	return Predicate{p: sqlbuild.Like(c.col, pattern)}
}

func ILike[T any](c Col[T, string], pattern string) Predicate {
	return Predicate{p: sqlbuild.ILike(c.col, pattern)}
}

func NotLike[T any](c Col[T, string], pattern string) Predicate {
	return Predicate{p: sqlbuild.NotLike(c.col, pattern)}
}
