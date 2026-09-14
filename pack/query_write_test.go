package pack

import (
	"context"
	"testing"

	"github.com/asimmons91/trails/pack/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryUpdate_RendersGoldenSQL_AndReturnsRowsAffected(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{RowsAffected: 2})

	n, err := Of[testAccount](db).
		Where(accountCol.ID.Eq(int64(1))).
		Update(context.Background(), accountCol.Email.Set("new@x.com"))
	require.NoError(t, err)
	assert.EqualValues(t, 2, n)

	executed := fake.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t,
		`UPDATE "accounts" AS "accounts" SET "email" = $1 WHERE "accounts"."id" = $2`,
		executed[0].SQL)
	assert.Equal(t, []any{"new@x.com", int64(1)}, executed[0].Args)
}

func TestQueryUpdate_NoWhere_UpdatesEveryRow(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{RowsAffected: 5})

	n, err := Of[testAccount](db).Update(context.Background(), accountCol.Email.Set("x@y.com"))
	require.NoError(t, err)
	assert.EqualValues(t, 5, n)

	executed := fake.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t, `UPDATE "accounts" AS "accounts" SET "email" = $1`, executed[0].SQL)
}

func TestQueryDelete_RendersGoldenSQL_AndReturnsRowsAffected(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{RowsAffected: 1})

	n, err := Of[testAccount](db).Where(accountCol.ID.Eq(int64(9))).Delete(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)

	executed := fake.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t, `DELETE FROM "accounts" AS "accounts" WHERE "accounts"."id" = $1`, executed[0].SQL)
	assert.Equal(t, []any{int64(9)}, executed[0].Args)
}

func TestQueryDelete_EmptyWhere_ReturnsErrorBeforeAnySQL(t *testing.T) {
	db, fake := newTestDB()

	_, err := Of[testAccount](db).Delete(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to delete with no WHERE condition")
	assert.Empty(t, fake.Executed())
}

func TestQueryDeleteAll_EmptyWhere_DeletesEveryRow(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{RowsAffected: 42})

	n, err := Of[testAccount](db).DeleteAll(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, 42, n)

	executed := fake.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t, `DELETE FROM "accounts" AS "accounts"`, executed[0].SQL)
}

func TestQueryDeleteAll_WithWhere_BehavesLikeDelete(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{RowsAffected: 1})

	n, err := Of[testAccount](db).Where(accountCol.ID.Eq(int64(3))).DeleteAll(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)

	executed := fake.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t, `DELETE FROM "accounts" AS "accounts" WHERE "accounts"."id" = $1`, executed[0].SQL)
}
