package sqlbuild

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Note: DeleteBuilder deliberately does not guard against a missing WHERE
// This file only tests rendering.

func TestDelete_Basic(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `DELETE FROM "users" WHERE "id" = $1`},
		{"SQLite", sqlite, `DELETE FROM "users" WHERE "id" = ?1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := Delete(Table{Name: "users"}).
				Where(Eq(Col("id"), 7)).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
			require.Equal(t, []any{7}, args)
		})
	}
}

func TestDelete_NoWhere_RendersWithoutWhereClause(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `DELETE FROM "users"`},
		{"SQLite", sqlite, `DELETE FROM "users"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := Delete(Table{Name: "users"}).Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
			require.Equal(t, []any{}, args)
		})
	}
}

func TestDelete_WithAlias_RendersASAliasAndAliasQualifiedWhere(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `DELETE FROM "users" AS "u" WHERE "u"."id" = $1`},
		{"SQLite", sqlite, `DELETE FROM "users" AS "u" WHERE "u"."id" = ?1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := Delete(Table{Name: "users", Alias: "u"}).
				Where(Eq(QualifiedCol("u", "id"), 7)).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
			require.Equal(t, []any{7}, args)
		})
	}
}
