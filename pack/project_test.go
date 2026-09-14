package pack

import (
	"context"
	"database/sql/driver"
	"testing"

	"github.com/asimmons91/trails/pack/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuery_Project_SameTable_NarrowerShape(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"email"},
		Rows:    [][]driver.Value{testdb.Row("a@b.com")},
	})

	got, err := Of[testAccount](db).
		Where(accountCol.ID.Eq(int64(7))).
		Project[testAccountSummary]().
		Find(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "a@b.com", got[0].Email)

	executed := fake.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t,
		`SELECT "accounts"."email" FROM "accounts" AS "accounts" WHERE "accounts"."id" = $1`,
		executed[0].SQL)
	assert.Equal(t, []any{int64(7)}, executed[0].Args)
}

func TestQuery_Project_AfterJoin_OntoJoinedTable(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "user_id", "title"},
		Rows:    [][]driver.Value{testdb.Row(int64(1), int64(7), "hello")},
	})

	got, err := Of[testAccount](db).
		Join[testPost](On(postCol.UserID, accountCol.ID)).
		Where(accountCol.ID.Eq(int64(7))).
		Project[testPost]().
		Find(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "hello", got[0].Title)

	executed := fake.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t,
		`SELECT "posts"."id", "posts"."user_id", "posts"."title" FROM "accounts" AS "accounts" JOIN "posts" AS "posts" ON "posts"."user_id" = "accounts"."id" WHERE "accounts"."id" = $1`,
		executed[0].SQL)
	assert.Equal(t, []any{int64(7)}, executed[0].Args)
}

func TestQuery_Project_CarriesOverOrderAndLimit(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"email"}})

	_, err := Of[testAccount](db).
		Order(accountCol.ID.Asc()).
		Limit(5).
		Project[testAccountSummary]().
		Find(context.Background())
	require.NoError(t, err)

	assert.Equal(t,
		`SELECT "accounts"."email" FROM "accounts" AS "accounts" ORDER BY "accounts"."id" ASC LIMIT 5`,
		fake.Executed()[0].SQL)
}
