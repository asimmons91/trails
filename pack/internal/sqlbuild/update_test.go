package sqlbuild

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpdate_SetAssignments(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `UPDATE "users" SET "email" = $1, "age" = $2 WHERE "id" = $3`},
		{"SQLite", sqlite, `UPDATE "users" SET "email" = ?1, "age" = ?2 WHERE "id" = ?3`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := Update(Table{Name: "users"}).
				Set(
					Set(Col("email"), "a@b.com"),
					Set(Col("age"), 31),
				).
				Where(Eq(Col("id"), 7)).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
			require.Equal(t, []any{"a@b.com", 31, 7}, args)
		})
	}
}

func TestUpdate_SetExpr_NoPlaceholderForValueItself(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `UPDATE "users" SET "login_count" = login_count + 1 WHERE "id" = $1`},
		{"SQLite", sqlite, `UPDATE "users" SET "login_count" = login_count + 1 WHERE "id" = ?1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := Update(Table{Name: "users"}).
				Set(SetExpr(Col("login_count"), "login_count + 1")).
				Where(Eq(Col("id"), 7)).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
			require.Equal(t, []any{7}, args)
		})
	}
}

func TestUpdate_MixedSetAndSetExpr_GlobalRenumbering(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `UPDATE "users" SET "email" = $1, "login_count" = login_count + $2 WHERE "id" = $3`},
		{"SQLite", sqlite, `UPDATE "users" SET "email" = ?1, "login_count" = login_count + ?2 WHERE "id" = ?3`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := Update(Table{Name: "users"}).
				Set(
					Set(Col("email"), "a@b.com"),
					SetExpr(Col("login_count"), "login_count + $1", 5),
				).
				Where(Eq(Col("id"), 7)).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
			require.Equal(t, []any{"a@b.com", 5, 7}, args)
		})
	}
}

func TestUpdate_NoWhere_RendersWithoutWhereClause(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `UPDATE "users" SET "active" = $1`},
		{"SQLite", sqlite, `UPDATE "users" SET "active" = ?1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, _, err := Update(Table{Name: "users"}).
				Set(Set(Col("active"), false)).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
		})
	}
}

func TestUpdate_WithAlias_RendersASAliasAndAliasQualifiedWhere(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `UPDATE "users" AS "u" SET "email" = $1 WHERE "u"."id" = $2`},
		{"SQLite", sqlite, `UPDATE "users" AS "u" SET "email" = ?1 WHERE "u"."id" = ?2`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := Update(Table{Name: "users", Alias: "u"}).
				Set(Set(Col("email"), "a@b.com")).
				Where(Eq(QualifiedCol("u", "id"), 7)).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
			require.Equal(t, []any{"a@b.com", 7}, args)
		})
	}
}

func TestUpdate_SetTarget_IsNeverQualified_EvenIfColumnCarriesAnAlias(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `UPDATE "users" AS "u" SET "email" = $1`},
		{"SQLite", sqlite, `UPDATE "users" AS "u" SET "email" = ?1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, _, err := Update(Table{Name: "users", Alias: "u"}).
				Set(Set(QualifiedCol("u", "email"), "a@b.com")).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
		})
	}
}
