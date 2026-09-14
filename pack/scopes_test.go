package pack

import (
	"context"
	"testing"

	"github.com/asimmons91/trails/pack/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func adultUsers(q *Query[testUser]) *Query[testUser] {
	return q.Where(userCol.Age.Gte(18))
}

func withEmail(email string) Scope[testUser] {
	return func(q *Query[testUser]) *Query[testUser] {
		return q.Where(userCol.Email.Eq(email))
	}
}

func TestQuery_Scopes_SingleScope_AppliesWhere(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"id", "email", "age"}})

	_, err := Of[testUser](db).
		Scopes(adultUsers).
		Find(context.Background())
	require.NoError(t, err)

	assert.Equal(t,
		`SELECT "users"."id", "users"."email", "users"."age" FROM "users" AS "users" WHERE "users"."age" >= $1`,
		fake.Executed()[0].SQL)
	assert.Equal(t, []any{18}, fake.Executed()[0].Args)
}

func TestQuery_Scopes_MultipleScopes_AreAndJoined(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"id", "email", "age"}})

	_, err := Of[testUser](db).
		Scopes(adultUsers, withEmail("a@b.com")).
		Find(context.Background())
	require.NoError(t, err)

	assert.Equal(t,
		`SELECT "users"."id", "users"."email", "users"."age" FROM "users" AS "users" WHERE "users"."age" >= $1 AND "users"."email" = $2`,
		fake.Executed()[0].SQL)
	assert.Equal(t, []any{18, "a@b.com"}, fake.Executed()[0].Args)
}

func TestQuery_Scopes_ComposeWithSurroundingChain(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"id", "email", "age"}})

	_, err := Of[testUser](db).
		Where(userCol.Email.IsNotNull()).
		Scopes(adultUsers).
		Order(userCol.Age.Desc()).
		Find(context.Background())
	require.NoError(t, err)

	assert.Equal(t,
		`SELECT "users"."id", "users"."email", "users"."age" FROM "users" AS "users" `+
			`WHERE "users"."email" IS NOT NULL AND "users"."age" >= $1 ORDER BY "users"."age" DESC`,
		fake.Executed()[0].SQL)
}

func TestQuery_Scopes_ParameterizedScope(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"id", "email", "age"}})

	_, err := Of[testUser](db).
		Scopes(withEmail("ada@example.com")).
		Find(context.Background())
	require.NoError(t, err)

	assert.Equal(t,
		`SELECT "users"."id", "users"."email", "users"."age" FROM "users" AS "users" WHERE "users"."email" = $1`,
		fake.Executed()[0].SQL)
	assert.Equal(t, []any{"ada@example.com"}, fake.Executed()[0].Args)
}

func TestQuery_Scopes_NoArgs_IsNoop(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"id", "email", "age"}})

	_, err := Of[testUser](db).
		Where(userCol.Age.Gt(18)).
		Scopes().
		Find(context.Background())
	require.NoError(t, err)

	assert.Equal(t,
		`SELECT "users"."id", "users"."email", "users"."age" FROM "users" AS "users" WHERE "users"."age" > $1`,
		fake.Executed()[0].SQL)
}
