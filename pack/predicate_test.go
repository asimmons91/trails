package pack

import (
	"context"
	"testing"

	"github.com/asimmons91/trails/pack/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func executedWhere(t *testing.T, p Predicate) (string, []any) {
	t.Helper()
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"id", "email", "age"}})

	_, err := Of[testUser](db).Where(p).Find(context.Background())
	require.NoError(t, err)

	executed := fake.Executed()
	require.Len(t, executed, 1)
	return executed[0].SQL, executed[0].Args
}

func TestPredicate_And(t *testing.T) {
	sql, args := executedWhere(t, And(
		userCol.Email.Eq("a@b.com"),
		userCol.Age.Gt(18),
	))
	assert.Equal(t,
		`SELECT "users"."id", "users"."email", "users"."age" FROM "users" AS "users" WHERE "users"."email" = $1 AND "users"."age" > $2`,
		sql)
	assert.Equal(t, []any{"a@b.com", 18}, args)
}

func TestPredicate_Or(t *testing.T) {
	sql, args := executedWhere(t, Or(
		userCol.Age.Lt(18),
		userCol.Age.Gt(65),
	))
	assert.Equal(t,
		`SELECT "users"."id", "users"."email", "users"."age" FROM "users" AS "users" WHERE "users"."age" < $1 OR "users"."age" > $2`,
		sql)
	assert.Equal(t, []any{18, 65}, args)
}

func TestPredicate_Not(t *testing.T) {
	sql, args := executedWhere(t, Not(And(
		userCol.Email.Eq("a@b.com"),
		userCol.Age.Gt(18),
	)))
	assert.Equal(t,
		`SELECT "users"."id", "users"."email", "users"."age" FROM "users" AS "users" WHERE NOT ("users"."email" = $1 AND "users"."age" > $2)`,
		sql)
	assert.Equal(t, []any{"a@b.com", 18}, args)
}

func TestPredicate_Like(t *testing.T) {
	sql, args := executedWhere(t, Like(userCol.Email, "a%"))
	assert.Equal(t,
		`SELECT "users"."id", "users"."email", "users"."age" FROM "users" AS "users" WHERE "users"."email" LIKE $1`,
		sql)
	assert.Equal(t, []any{"a%"}, args)
}

func TestPredicate_ILike(t *testing.T) {
	sql, args := executedWhere(t, ILike(userCol.Email, "a%"))
	assert.Equal(t,
		`SELECT "users"."id", "users"."email", "users"."age" FROM "users" AS "users" WHERE "users"."email" ILIKE $1`,
		sql)
	assert.Equal(t, []any{"a%"}, args)
}

func TestPredicate_NotLike(t *testing.T) {
	sql, args := executedWhere(t, NotLike(userCol.Email, "a%"))
	assert.Equal(t,
		`SELECT "users"."id", "users"."email", "users"."age" FROM "users" AS "users" WHERE "users"."email" NOT LIKE $1`,
		sql)
	assert.Equal(t, []any{"a%"}, args)
}
