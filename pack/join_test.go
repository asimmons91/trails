package pack

import (
	"context"
	"testing"

	"github.com/asimmons91/trails/pack/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuery_Join_RendersInnerJoinAndKeepsDefaultColumns(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "email", "created_at", "nickname"},
	})

	_, err := Of[testAccount](db).
		Join[testPost](On(postCol.UserID, accountCol.ID)).
		Where(postCol.Title.Eq("hello")).
		Find(context.Background())
	require.NoError(t, err)

	executed := fake.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t,
		`SELECT "accounts"."id", "accounts"."email", "accounts"."created_at", "accounts"."nickname" FROM "accounts" AS "accounts" JOIN "posts" AS "posts" ON "posts"."user_id" = "accounts"."id" WHERE "posts"."title" = $1`,
		executed[0].SQL)
	assert.Equal(t, []any{"hello"}, executed[0].Args)
}

func TestQuery_LeftJoin_RendersLeftJoin(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "email", "created_at", "nickname"},
	})

	_, err := Of[testAccount](db).
		LeftJoin[testPost](On(postCol.UserID, accountCol.ID)).
		Find(context.Background())
	require.NoError(t, err)

	assert.Equal(t,
		`SELECT "accounts"."id", "accounts"."email", "accounts"."created_at", "accounts"."nickname" FROM "accounts" AS "accounts" LEFT JOIN "posts" AS "posts" ON "posts"."user_id" = "accounts"."id"`,
		fake.Executed()[0].SQL)
}

func TestQuery_MultipleJoins_RenderInCallOrder(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "email", "created_at", "nickname"},
	})

	_, err := Of[testAccount](db).
		Join[testPost](On(postCol.UserID, accountCol.ID)).
		LeftJoin[testProfile](On(profileCol.AccountID, accountCol.ID)).
		Find(context.Background())
	require.NoError(t, err)

	assert.Equal(t,
		`SELECT "accounts"."id", "accounts"."email", "accounts"."created_at", "accounts"."nickname" FROM "accounts" AS "accounts" JOIN "posts" AS "posts" ON "posts"."user_id" = "accounts"."id" LEFT JOIN "profiles" AS "profiles" ON "profiles"."account_id" = "accounts"."id"`,
		fake.Executed()[0].SQL)
}
