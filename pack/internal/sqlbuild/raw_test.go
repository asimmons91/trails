package sqlbuild

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRaw_SingleFragment_RenumbersPlaceholders(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `SELECT * FROM "users" AS "u" WHERE "age" > $1 AND "age" < $2`},
		{"SQLite", sqlite, `SELECT * FROM "users" AS "u" WHERE "age" > ?1 AND "age" < ?2`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := Select(users).
				Where(Raw(`"age" > $1 AND "age" < $2`, 18, 65)).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
			require.Equal(t, []any{18, 65}, args)
		})
	}
}

func TestRaw_OutOfRangePlaceholder_IsError(t *testing.T) {
	_, _, err := Select(users).
		Where(Raw(`"age" > $2`, 18)).
		Render(pg)
	require.Error(t, err)
	require.IsType(t, &ErrRawPlaceholderOutOfRange{}, err)
}

func TestRaw_ZeroPlaceholder_IsError(t *testing.T) {
	_, _, err := Select(users).
		Where(Raw(`"age" > $0`, 18)).
		Render(pg)
	require.Error(t, err)
	require.IsType(t, &ErrRawPlaceholderOutOfRange{}, err)
}

func TestRaw_TwoFragmentsInOneStatement_ThreadGlobalCounter(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `UPDATE "users" SET "login_count" = login_count + $1 WHERE "age" > $2`},
		{"SQLite", sqlite, `UPDATE "users" SET "login_count" = login_count + ?1 WHERE "age" > ?2`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, args, err := Update(Table{Name: "users"}).
				Set(SetExpr(Col("login_count"), "login_count + $1", 1)).
				Where(Raw(`"age" > $1`, 18)).
				Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
			require.Equal(t, []any{1, 18}, args)
		})
	}
}
