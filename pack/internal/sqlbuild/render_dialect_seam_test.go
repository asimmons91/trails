package sqlbuild

import (
	"strconv"
	"strings"
	"testing"

	"github.com/asimmons91/trails/pack/dialect"
	"github.com/stretchr/testify/require"
)

// fakeDialect uses backtick quoting and "?N" placeholders — nothing like
// Postgres — to prove that internal/sqlbuild never hard-codes Postgres
// syntax. If this test needed to change sqlbuild itself to pass,
// that would mean Postgres-specific rendering had leaked out of dialect.
type fakeDialect struct{}

func (fakeDialect) Name() string { return "fake" }
func (fakeDialect) QuoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}
func (fakeDialect) Placeholder(n int) string { return "?" + strconv.Itoa(n) }
func (fakeDialect) SupportsILike() bool      { return true }
func (fakeDialect) SupportsRowLocking() bool { return true }

func TestRender_DifferentDialect_ChangesSyntaxOnlyNotStructure(t *testing.T) {
	build := func(d dialect.Dialect) (string, []any, error) {
		return Select(users).
			Where(Eq(Col("email"), "a@b.com")).
			Where(Gt(Col("age"), 18)).
			Render(d)
	}

	pgSQL, pgArgs, err := build(pg)
	require.NoError(t, err)
	require.Equal(t, `SELECT * FROM "users" AS "u" WHERE "email" = $1 AND "age" > $2`, pgSQL)

	fakeSQL, fakeArgs, err := build(fakeDialect{})
	require.NoError(t, err)
	require.Equal(t, "SELECT * FROM `users` AS `u` WHERE `email` = ?1 AND `age` > ?2", fakeSQL)

	// Same arguments, same structure — only quoting/placeholder syntax differs.
	require.Equal(t, pgArgs, fakeArgs)
}
