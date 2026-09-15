package sqlbuild

import (
	"testing"

	"github.com/asimmons91/trails/pack/dialect"
	"github.com/asimmons91/trails/pack/dialect/mysqldialect"
	"github.com/asimmons91/trails/pack/dialect/pgdialect"
	"github.com/asimmons91/trails/pack/dialect/sqlitedialect"
	"github.com/stretchr/testify/require"
)

var pg = pgdialect.New()
var sqlite = sqlitedialect.New()
var mysql = mysqldialect.New()

var users = Table{Name: "users", Alias: "u"}

// dialectCase pairs a dialect with the literal SQL a test expects it to
// produce. Every parameterized test builds its own slice — wantSQL is
// always a fully spelled-out string, never generated from a placeholder
// helper.
type dialectCase struct {
	name    string
	d       dialect.Dialect
	wantSQL string
}

func TestSelect_NoColumns_EmitsStar(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `SELECT * FROM "users" AS "u"`},
		{"SQLite", sqlite, `SELECT * FROM "users" AS "u"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := Select(users).Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
			require.Equal(t, []any{}, args)
		})
	}
}

func TestSelect_ExplicitColumns(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `SELECT "u"."id", "u"."email" FROM "users" AS "u"`},
		{"SQLite", sqlite, `SELECT "u"."id", "u"."email" FROM "users" AS "u"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := Select(users).
				Columns(QualifiedCol("u", "id"), QualifiedCol("u", "email")).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
			require.Equal(t, []any{}, args)
		})
	}
}

func TestSelect_Distinct(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `SELECT DISTINCT * FROM "users" AS "u"`},
		{"SQLite", sqlite, `SELECT DISTINCT * FROM "users" AS "u"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, _, err := Select(users).Distinct().Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
		})
	}
}

func TestSelect_TableWithoutAlias_OmitsAS(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `SELECT * FROM "users"`},
		{"SQLite", sqlite, `SELECT * FROM "users"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, _, err := Select(Table{Name: "users"}).Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
		})
	}
}

func TestSelect_ComparisonOperators(t *testing.T) {
	age := Col("age")
	cases := []struct {
		name       string
		pred       Predicate
		wantPG     string
		wantSQLite string
	}{
		{"Eq", Eq(age, 18), `"age" = $1`, `"age" = ?1`},
		{"Ne", Ne(age, 18), `"age" <> $1`, `"age" <> ?1`},
		{"Gt", Gt(age, 18), `"age" > $1`, `"age" > ?1`},
		{"Gte", Gte(age, 18), `"age" >= $1`, `"age" >= ?1`},
		{"Lt", Lt(age, 18), `"age" < $1`, `"age" < ?1`},
		{"Lte", Lte(age, 18), `"age" <= $1`, `"age" <= ?1`},
	}
	for _, tc := range cases {
		for _, dc := range []dialectCase{
			{"Postgres", pg, tc.wantPG},
			{"SQLite", sqlite, tc.wantSQLite},
		} {
			t.Run(tc.name+"/"+dc.name, func(t *testing.T) {
				sql, args, err := Select(users).Where(tc.pred).Render(dc.d)
				require.NoError(t, err)
				require.Equal(t, `SELECT * FROM "users" AS "u" WHERE `+dc.wantSQL, sql)
				require.Equal(t, []any{18}, args)
			})
		}
	}
}

func TestSelect_In_WithValues(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `SELECT * FROM "users" AS "u" WHERE "id" IN ($1, $2, $3)`},
		{"SQLite", sqlite, `SELECT * FROM "users" AS "u" WHERE "id" IN (?1, ?2, ?3)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := Select(users).
				Where(In(Col("id"), 1, 2, 3)).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
			require.Equal(t, []any{1, 2, 3}, args)
		})
	}
}

func TestSelect_In_NoValues_RendersFalse(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `SELECT * FROM "users" AS "u" WHERE FALSE`},
		{"SQLite", sqlite, `SELECT * FROM "users" AS "u" WHERE FALSE`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := Select(users).
				Where(In(Col("id"))).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
			require.Equal(t, []any{}, args)
		})
	}
}

func TestSelect_NotIn_WithValues(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `SELECT * FROM "users" AS "u" WHERE "id" NOT IN ($1, $2)`},
		{"SQLite", sqlite, `SELECT * FROM "users" AS "u" WHERE "id" NOT IN (?1, ?2)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := Select(users).
				Where(NotIn(Col("id"), 1, 2)).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
			require.Equal(t, []any{1, 2}, args)
		})
	}
}

func TestSelect_NotIn_NoValues_RendersTrue(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `SELECT * FROM "users" AS "u" WHERE TRUE`},
		{"SQLite", sqlite, `SELECT * FROM "users" AS "u" WHERE TRUE`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := Select(users).
				Where(NotIn(Col("id"))).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
			require.Equal(t, []any{}, args)
		})
	}
}

func TestSelect_IsNull_IsNotNull(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `SELECT * FROM "users" AS "u" WHERE "deleted_at" IS NULL`},
		{"SQLite", sqlite, `SELECT * FROM "users" AS "u" WHERE "deleted_at" IS NULL`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := Select(users).
				Where(IsNull(Col("deleted_at"))).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
			require.Equal(t, []any{}, args)
		})
	}

	for _, tc := range []dialectCase{
		{"Postgres", pg, `SELECT * FROM "users" AS "u" WHERE "deleted_at" IS NOT NULL`},
		{"SQLite", sqlite, `SELECT * FROM "users" AS "u" WHERE "deleted_at" IS NOT NULL`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := Select(users).
				Where(IsNotNull(Col("deleted_at"))).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
			require.Equal(t, []any{}, args)
		})
	}
}

func TestSelect_Between(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `SELECT * FROM "users" AS "u" WHERE "age" BETWEEN $1 AND $2`},
		{"SQLite", sqlite, `SELECT * FROM "users" AS "u" WHERE "age" BETWEEN ?1 AND ?2`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := Select(users).
				Where(Between(Col("age"), 18, 65)).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
			require.Equal(t, []any{18, 65}, args)
		})
	}
}

func TestSelect_LikeFamily(t *testing.T) {
	email := Col("email")
	cases := []struct {
		name       string
		pred       Predicate
		wantPG     string
		wantSQLite string
	}{
		{"Like", Like(email, "a%"), `"email" LIKE $1`, `"email" LIKE ?1`},
		{"NotLike", NotLike(email, "a%"), `"email" NOT LIKE $1`, `"email" NOT LIKE ?1`},
	}
	for _, tc := range cases {
		for _, dc := range []dialectCase{
			{"Postgres", pg, tc.wantPG},
			{"SQLite", sqlite, tc.wantSQLite},
		} {
			t.Run(tc.name+"/"+dc.name, func(t *testing.T) {
				sql, args, err := Select(users).Where(tc.pred).Render(dc.d)
				require.NoError(t, err)
				require.Equal(t, `SELECT * FROM "users" AS "u" WHERE `+dc.wantSQL, sql)
				require.Equal(t, []any{"a%"}, args)
			})
		}
	}
}

func TestSelect_ILike_Postgres(t *testing.T) {
	sql, args, err := Select(users).
		Where(ILike(Col("email"), "a%")).
		Render(pg)
	require.NoError(t, err)
	require.Equal(t, `SELECT * FROM "users" AS "u" WHERE "email" ILIKE $1`, sql)
	require.Equal(t, []any{"a%"}, args)
}

func TestSelect_ILike_UnsupportedBySQLite(t *testing.T) {
	_, _, err := Select(users).
		Where(ILike(Col("email"), "a%")).
		Render(sqlite)
	require.Error(t, err)
	require.IsType(t, &ErrILikeUnsupportedByDialect{}, err)
}

func TestWhere_ChainedCalls_AreAndJoined(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `SELECT * FROM "users" AS "u" WHERE "email" = $1 AND "age" > $2`},
		{"SQLite", sqlite, `SELECT * FROM "users" AS "u" WHERE "email" = ?1 AND "age" > ?2`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := Select(users).
				Where(Eq(Col("email"), "a@b.com")).
				Where(Gt(Col("age"), 18)).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
			require.Equal(t, []any{"a@b.com", 18}, args)
		})
	}
}

func TestAndOr_ComposeArbitrarily_WithDeterministicParens(t *testing.T) {
	pred := And(
		Eq(Col("active"), true),
		Or(
			Gt(Col("age"), 65),
			Lt(Col("age"), 18),
		),
	)
	for _, tc := range []dialectCase{
		{"Postgres", pg, `SELECT * FROM "users" AS "u" WHERE "active" = $1 AND ("age" > $2 OR "age" < $3)`},
		{"SQLite", sqlite, `SELECT * FROM "users" AS "u" WHERE "active" = ?1 AND ("age" > ?2 OR "age" < ?3)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := Select(users).Where(pred).Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
			require.Equal(t, []any{true, 65, 18}, args)
		})
	}
}

func TestNot_WrapsChildInParensWhenCombinator(t *testing.T) {
	pred := Not(And(
		Eq(Col("a"), 1),
		Eq(Col("b"), 2),
	))
	for _, tc := range []dialectCase{
		{"Postgres", pg, `SELECT * FROM "users" AS "u" WHERE NOT ("a" = $1 AND "b" = $2)`},
		{"SQLite", sqlite, `SELECT * FROM "users" AS "u" WHERE NOT ("a" = ?1 AND "b" = ?2)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := Select(users).Where(pred).Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
			require.Equal(t, []any{1, 2}, args)
		})
	}
}

func TestNot_LeafPredicate_NoExtraParens(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `SELECT * FROM "users" AS "u" WHERE NOT "a" = $1`},
		{"SQLite", sqlite, `SELECT * FROM "users" AS "u" WHERE NOT "a" = ?1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, _, err := Select(users).Where(Not(Eq(Col("a"), 1))).Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
		})
	}
}

func TestAnd_Empty_RendersTrue(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `SELECT * FROM "users" AS "u" WHERE TRUE`},
		{"SQLite", sqlite, `SELECT * FROM "users" AS "u" WHERE TRUE`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := Select(users).Where(And()).Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
			require.Equal(t, []any{}, args)
		})
	}
}

func TestOr_Empty_RendersFalse(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `SELECT * FROM "users" AS "u" WHERE FALSE`},
		{"SQLite", sqlite, `SELECT * FROM "users" AS "u" WHERE FALSE`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := Select(users).Where(Or()).Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
			require.Equal(t, []any{}, args)
		})
	}
}

func TestSelect_GroupByAndHaving(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `SELECT * FROM "users" AS "u" GROUP BY "country" HAVING "count" > $1`},
		{"SQLite", sqlite, `SELECT * FROM "users" AS "u" GROUP BY "country" HAVING "count" > ?1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := Select(users).
				GroupBy(Col("country")).
				Having(Gt(Col("count"), 10)).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
			require.Equal(t, []any{10}, args)
		})
	}
}

func TestSelect_OrderBy_AscDesc_MultipleTerms(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `SELECT * FROM "users" AS "u" ORDER BY "email" ASC, "age" DESC`},
		{"SQLite", sqlite, `SELECT * FROM "users" AS "u" ORDER BY "email" ASC, "age" DESC`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, _, err := Select(users).
				OrderBy(Asc(Col("email")), Desc(Col("age"))).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
		})
	}
}

func TestSelect_OrderRaw_AppendsVerbatim(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `SELECT * FROM "users" AS "u" ORDER BY random()`},
		{"SQLite", sqlite, `SELECT * FROM "users" AS "u" ORDER BY random()`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, _, err := Select(users).
				OrderBy(OrderRaw("random()")).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
		})
	}
}

func TestSelect_LimitOffset(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `SELECT * FROM "users" AS "u" LIMIT 10 OFFSET 20`},
		{"SQLite", sqlite, `SELECT * FROM "users" AS "u" LIMIT 10 OFFSET 20`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, _, err := Select(users).Limit(10).Offset(20).Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
		})
	}
}

func TestSelect_ForUpdate(t *testing.T) {
	sql, _, err := Select(users).ForUpdate().Render(pg)
	require.NoError(t, err)
	require.Equal(t, `SELECT * FROM "users" AS "u" FOR UPDATE`, sql)
}

func TestSelect_ForUpdate_UnsupportedBySQLite(t *testing.T) {
	_, _, err := Select(users).ForUpdate().Render(sqlite)
	require.Error(t, err)
	require.IsType(t, &ErrRowLockingUnsupportedByDialect{}, err)
}

func TestSelect_ForShare(t *testing.T) {
	sql, _, err := Select(users).ForShare().Render(pg)
	require.NoError(t, err)
	require.Equal(t, `SELECT * FROM "users" AS "u" FOR SHARE`, sql)
}

func TestSelect_ForShare_UnsupportedBySQLite(t *testing.T) {
	_, _, err := Select(users).ForShare().Render(sqlite)
	require.Error(t, err)
	require.IsType(t, &ErrRowLockingUnsupportedByDialect{}, err)
}

func TestSelect_ForUpdateAndForShare_IsError(t *testing.T) {
	_, _, err := Select(users).ForUpdate().ForShare().Render(pg)
	require.Error(t, err)
	require.IsType(t, &ErrConflictingLockClause{}, err)
}

func TestSelect_ForUpdateSkipLocked(t *testing.T) {
	sql, _, err := Select(users).ForUpdate().SkipLocked().Render(pg)
	require.NoError(t, err)
	require.Equal(t, `SELECT * FROM "users" AS "u" FOR UPDATE SKIP LOCKED`, sql)
}

func TestSelect_ForShareSkipLocked(t *testing.T) {
	sql, _, err := Select(users).ForShare().SkipLocked().Render(pg)
	require.NoError(t, err)
	require.Equal(t, `SELECT * FROM "users" AS "u" FOR SHARE SKIP LOCKED`, sql)
}

func TestSelect_SkipLockedWithoutLockClause_IsError(t *testing.T) {
	_, _, err := Select(users).SkipLocked().Render(pg)
	require.Error(t, err)
	require.IsType(t, &ErrSkipLockedRequiresLockClause{}, err)
}

func TestSelect_ForUpdateSkipLocked_UnsupportedBySQLite(t *testing.T) {
	_, _, err := Select(users).ForUpdate().SkipLocked().Render(sqlite)
	require.Error(t, err)
	require.IsType(t, &ErrRowLockingUnsupportedByDialect{}, err)
}

func TestSelect_SelectRaw_AppendsToColumnList(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `SELECT "id", count(*) OVER () FROM "users" AS "u"`},
		{"SQLite", sqlite, `SELECT "id", count(*) OVER () FROM "users" AS "u"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, _, err := Select(users).
				Columns(Col("id")).
				SelectRaw("count(*) OVER ()").
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
		})
	}
}

func TestSelect_WhereRawCombinedWithBuilderPredicate_RenumbersAcrossFragments(t *testing.T) {
	pred := And(
		Eq(Col("active"), true),
		Raw(`"age" > $1`, 21),
	)
	for _, tc := range []dialectCase{
		{"Postgres", pg, `SELECT * FROM "users" AS "u" WHERE "active" = $1 AND "age" > $2`},
		{"SQLite", sqlite, `SELECT * FROM "users" AS "u" WHERE "active" = ?1 AND "age" > ?2`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := Select(users).Where(pred).Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
			require.Equal(t, []any{true, 21}, args)
		})
	}
}

func TestSelect_QualifiedColumn_RendersTableDotColumn(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `SELECT * FROM "users" AS "u" WHERE "u"."email" = $1`},
		{"SQLite", sqlite, `SELECT * FROM "users" AS "u" WHERE "u"."email" = ?1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, _, err := Select(users).
				Where(Eq(QualifiedCol("u", "email"), "a@b.com")).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
		})
	}
}

func TestSelect_IdentifierWithEmbeddedQuote_IsDoubled(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `SELECT * FROM "wei""rd"`},
		{"SQLite", sqlite, `SELECT * FROM "wei""rd"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, _, err := Select(Table{Name: `wei"rd`}).Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
		})
	}
}

func TestSelect_Determinism_SameConstruction_ProducesByteIdenticalSQL(t *testing.T) {
	for _, tc := range []struct {
		name string
		d    dialect.Dialect
	}{
		{"Postgres", pg},
		{"SQLite", sqlite},
	} {
		t.Run(tc.name, func(t *testing.T) {
			build := func() (string, []any, error) {
				return Select(users).
					Where(Eq(Col("email"), "a@b.com")).
					Where(Gt(Col("age"), 18)).
					OrderBy(Asc(Col("email"))).
					Limit(5).
					Render(tc.d)
			}
			sql1, args1, err1 := build()
			require.NoError(t, err1)
			sql2, args2, err2 := build()
			require.NoError(t, err2)
			require.Equal(t, sql1, sql2)
			require.Equal(t, args1, args2)

			// Same logical WHERE, built via a single And() instead of two chained
			// Where() calls, must also match byte-for-byte.
			sql3, args3, err3 := Select(users).
				Where(And(
					Eq(Col("email"), "a@b.com"),
					Gt(Col("age"), 18),
				)).
				OrderBy(Asc(Col("email"))).
				Limit(5).
				Render(tc.d)
			require.NoError(t, err3)
			require.Equal(t, sql1, sql3)
			require.Equal(t, args1, args3)
		})
	}
}

func TestSelect_BuiltOnTwice_DoesNotLeakBetweenDerivedBuilders(t *testing.T) {
	for _, tc := range []struct {
		name string
		d    dialect.Dialect
	}{
		{"Postgres", pg},
		{"SQLite", sqlite},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := Select(users).Where(Gt(Col("age"), 18))

			withOrder, _, err := base.OrderBy(Asc(Col("email"))).Render(tc.d)
			require.NoError(t, err)

			baseAgain, _, err := base.Render(tc.d)
			require.NoError(t, err)

			// Deriving twice from the same base must not have the two derivations
			// interfere with each other either.
			withCols, _, err := base.Columns(Col("id")).Render(tc.d)
			require.NoError(t, err)

			baseFinal, _, err := base.Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, baseAgain, baseFinal)

			switch tc.name {
			case "Postgres":
				require.Equal(t, `SELECT * FROM "users" AS "u" WHERE "age" > $1`, baseAgain)
				require.Equal(t, `SELECT * FROM "users" AS "u" WHERE "age" > $1 ORDER BY "email" ASC`, withOrder)
				require.Equal(t, `SELECT "id" FROM "users" AS "u" WHERE "age" > $1`, withCols)
			case "SQLite":
				require.Equal(t, `SELECT * FROM "users" AS "u" WHERE "age" > ?1`, baseAgain)
				require.Equal(t, `SELECT * FROM "users" AS "u" WHERE "age" > ?1 ORDER BY "email" ASC`, withOrder)
				require.Equal(t, `SELECT "id" FROM "users" AS "u" WHERE "age" > ?1`, withCols)
			}
		})
	}
}

var posts = Table{Name: "posts", Alias: "p"}

func TestSelect_Join_RendersInnerJoinWithOnCondition(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `SELECT * FROM "users" AS "u" JOIN "posts" AS "p" ON "p"."user_id" = "u"."id" WHERE "p"."published" = $1`},
		{"SQLite", sqlite, `SELECT * FROM "users" AS "u" JOIN "posts" AS "p" ON "p"."user_id" = "u"."id" WHERE "p"."published" = ?1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := Select(users).
				Join(posts, EqCol(QualifiedCol("p", "user_id"), QualifiedCol("u", "id"))).
				Where(Eq(QualifiedCol("p", "published"), true)).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
			require.Equal(t, []any{true}, args)
		})
	}
}

func TestSelect_LeftJoin_RendersLeftJoin(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `SELECT * FROM "users" AS "u" LEFT JOIN "posts" AS "p" ON "p"."user_id" = "u"."id"`},
		{"SQLite", sqlite, `SELECT * FROM "users" AS "u" LEFT JOIN "posts" AS "p" ON "p"."user_id" = "u"."id"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, _, err := Select(users).
				LeftJoin(posts, EqCol(QualifiedCol("p", "user_id"), QualifiedCol("u", "id"))).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
		})
	}
}

func TestSelect_MultipleJoins_RenderInCallOrder(t *testing.T) {
	comments := Table{Name: "comments", Alias: "c"}
	for _, tc := range []dialectCase{
		{"Postgres", pg, `SELECT * FROM "users" AS "u" JOIN "posts" AS "p" ON "p"."user_id" = "u"."id" LEFT JOIN "comments" AS "c" ON "c"."post_id" = "p"."id"`},
		{"SQLite", sqlite, `SELECT * FROM "users" AS "u" JOIN "posts" AS "p" ON "p"."user_id" = "u"."id" LEFT JOIN "comments" AS "c" ON "c"."post_id" = "p"."id"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, _, err := Select(users).
				Join(posts, EqCol(QualifiedCol("p", "user_id"), QualifiedCol("u", "id"))).
				LeftJoin(comments, EqCol(QualifiedCol("c", "post_id"), QualifiedCol("p", "id"))).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
		})
	}
}

func TestSelect_EqCol_NoPlaceholderConsumed(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `SELECT * FROM "users" AS "u" JOIN "posts" AS "p" ON "p"."user_id" = "u"."id"`},
		{"SQLite", sqlite, `SELECT * FROM "users" AS "u" JOIN "posts" AS "p" ON "p"."user_id" = "u"."id"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := Select(users).
				Join(posts, EqCol(QualifiedCol("p", "user_id"), QualifiedCol("u", "id"))).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
			require.Equal(t, []any{}, args)
		})
	}
}

func TestSelect_Join_Determinism(t *testing.T) {
	for _, tc := range []struct {
		name string
		d    dialect.Dialect
	}{
		{"Postgres", pg},
		{"SQLite", sqlite},
	} {
		t.Run(tc.name, func(t *testing.T) {
			build := func() (string, []any, error) {
				return Select(users).
					Join(posts, EqCol(QualifiedCol("p", "user_id"), QualifiedCol("u", "id"))).
					Where(Eq(QualifiedCol("p", "published"), true)).
					Render(tc.d)
			}
			sql1, args1, err1 := build()
			require.NoError(t, err1)
			sql2, args2, err2 := build()
			require.NoError(t, err2)
			require.Equal(t, sql1, sql2)
			require.Equal(t, args1, args2)
		})
	}
}
