package pack

import "testing"

// BenchmarkColEq_SinglePredicate measures building one predicate from a
// Col handle — the actual user-facing
// surface, isolated from SQL rendering, which is internal/sqlbuild's own
// concern and separately benchmarked.
func BenchmarkColEq_SinglePredicate(b *testing.B) {
	for b.Loop() {
		_ = userCol.Age.Gt(18)
	}
}

// BenchmarkWhere_ChainedPredicates measures composing several predicates
// onto a Query[T] via successive Where calls (andJoin, query.go) — the cost
// of building up a WHERE condition before it is ever rendered.
func BenchmarkWhere_ChainedPredicates(b *testing.B) {
	db, _ := newTestDB()
	for b.Loop() {
		_ = Of[testUser](db).
			Where(userCol.Age.Gt(18)).
			Where(userCol.Email.Eq("a@b.com")).
			Where(userCol.ID.Ne(0))
	}
}
