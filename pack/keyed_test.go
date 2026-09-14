package pack

import (
	"context"
	"database/sql/driver"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/asimmons91/trails/pack/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreate_AutoincrementAndDefaultColumn_ReturningWriteBack(t *testing.T) {
	db, fake := newTestDB()
	createdAt := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "created_at"},
		Rows:    [][]driver.Value{testdb.Row(int64(42), createdAt)},
	})

	row := &testAccount{Email: "a@b.com"}
	err := Create(context.Background(), db, row)
	require.NoError(t, err)

	executed := fake.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t,
		`INSERT INTO "accounts" ("email", "nickname") VALUES ($1, $2) RETURNING "id", "created_at"`,
		executed[0].SQL)
	assert.Equal(t, []any{"a@b.com", nil}, executed[0].Args)

	assert.EqualValues(t, 42, row.ID)
	assert.True(t, row.CreatedAt.Equal(createdAt))
}

func TestCreate_DefaultColumnGivenNonZeroValue_IsInsertedNotOmitted(t *testing.T) {
	db, fake := newTestDB()
	explicit := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	fake.Enqueue(testdb.Result{
		Columns: []string{"id"},
		Rows:    [][]driver.Value{testdb.Row(int64(1))},
	})

	row := &testAccount{Email: "a@b.com", CreatedAt: explicit}
	err := Create(context.Background(), db, row)
	require.NoError(t, err)

	assert.Equal(t,
		`INSERT INTO "accounts" ("email", "created_at", "nickname") VALUES ($1, $2, $3) RETURNING "id"`,
		fake.Executed()[0].SQL)
	assert.Equal(t, []any{"a@b.com", explicit, nil}, fake.Executed()[0].Args)
}

func TestCreate_CompositeKey_NoReturning_UsesExecContext(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{RowsAffected: 1})

	row := &testMembership{
		Model: Model[testMemberKey]{ID: testMemberKey{OrgID: 1, UserID: 2}},
		Role:  "admin",
	}
	err := Create(context.Background(), db, row)
	require.NoError(t, err)

	executed := fake.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t,
		`INSERT INTO "memberships" ("org_id", "user_id", "role") VALUES ($1, $2, $3)`,
		executed[0].SQL)
	assert.Equal(t, []any{int64(1), int64(2), "admin"}, executed[0].Args)
}

func TestCreate_ZeroCompositeKey_ReturnsErrorBeforeAnySQL(t *testing.T) {
	db, fake := newTestDB()

	row := &testMembership{Role: "admin"}
	err := Create(context.Background(), db, row)
	require.Error(t, err)
	var zErr *ErrZeroCompositeKey
	require.True(t, errors.As(err, &zErr))
	assert.Empty(t, fake.Executed())
}

func TestCreate_UintOverflow_ReturnsErrorBeforeAnySQL(t *testing.T) {
	db, fake := newTestDB()

	row := &testWidget{Count: uint64(math.MaxInt64) + 1}
	err := Create(context.Background(), db, row)
	require.Error(t, err)
	var oErr *ErrUintOverflow
	require.True(t, errors.As(err, &oErr))
	assert.Equal(t, "Count", oErr.Field)
	assert.Equal(t, "count", oErr.Column)
	assert.Equal(t, uint64(math.MaxInt64)+1, oErr.Value)
	assert.Empty(t, fake.Executed())
}

func TestByID_ScalarKey_RendersSelectWithPKPredicate(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "email", "created_at", "nickname"},
		Rows:    [][]driver.Value{testdb.Row(int64(7), "a@b.com", time.Time{}, "")},
	})

	got, err := ByID[testAccount](context.Background(), db, int64(7))
	require.NoError(t, err)
	assert.EqualValues(t, 7, got.ID)

	executed := fake.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t,
		`SELECT "accounts"."id", "accounts"."email", "accounts"."created_at", "accounts"."nickname" FROM "accounts" AS "accounts" WHERE "accounts"."id" = $1 LIMIT 1`,
		executed[0].SQL)
	assert.Equal(t, []any{int64(7)}, executed[0].Args)
}

func TestByID_CompositeKey_RendersANDedPredicate(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"org_id", "user_id", "role"},
		Rows:    [][]driver.Value{testdb.Row(int64(1), int64(2), "admin")},
	})

	got, err := ByID[testMembership](context.Background(), db, testMemberKey{OrgID: 1, UserID: 2})
	require.NoError(t, err)
	assert.Equal(t, "admin", got.Role)

	executed := fake.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t,
		`SELECT "memberships"."org_id", "memberships"."user_id", "memberships"."role" FROM "memberships" AS "memberships" WHERE "memberships"."org_id" = $1 AND "memberships"."user_id" = $2 LIMIT 1`,
		executed[0].SQL)
	assert.Equal(t, []any{int64(1), int64(2)}, executed[0].Args)
}

func TestByID_NoRows_ReturnsErrNoRows(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"id", "email", "created_at", "nickname"}})

	_, err := ByID[testAccount](context.Background(), db, int64(7))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNoRows)
}

func TestUpdate_ScalarKey_WritesEveryColumnExceptPK(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{RowsAffected: 1})

	createdAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	row := &testAccount{
		Model:     Model[int64]{ID: 7},
		Email:     "a@b.com",
		CreatedAt: createdAt,
		Nickname:  "bob",
	}
	err := Update(context.Background(), db, row)
	require.NoError(t, err)

	executed := fake.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t,
		`UPDATE "accounts" SET "email" = $1, "created_at" = $2, "nickname" = $3 WHERE "id" = $4`,
		executed[0].SQL)
	assert.Equal(t, []any{"a@b.com", createdAt, "bob", int64(7)}, executed[0].Args)
}

func TestUpdate_CompositeKey_ANDedWhere(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{RowsAffected: 1})

	row := &testMembership{
		Model: Model[testMemberKey]{ID: testMemberKey{OrgID: 1, UserID: 2}},
		Role:  "admin",
	}
	err := Update(context.Background(), db, row)
	require.NoError(t, err)

	executed := fake.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t,
		`UPDATE "memberships" SET "role" = $1 WHERE "org_id" = $2 AND "user_id" = $3`,
		executed[0].SQL)
	assert.Equal(t, []any{"admin", int64(1), int64(2)}, executed[0].Args)
}

func TestUpdate_UintOverflow_ReturnsErrorBeforeAnySQL(t *testing.T) {
	db, fake := newTestDB()

	row := &testWidget{Model: Model[int64]{ID: 1}, Count: uint64(math.MaxInt64) + 1}
	err := Update(context.Background(), db, row)
	require.Error(t, err)
	var oErr *ErrUintOverflow
	require.True(t, errors.As(err, &oErr))
	assert.Empty(t, fake.Executed())
}

func TestDelete_ScalarKey(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{RowsAffected: 1})

	err := Delete[testAccount](context.Background(), db, int64(7))
	require.NoError(t, err)

	executed := fake.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t, `DELETE FROM "accounts" WHERE "id" = $1`, executed[0].SQL)
	assert.Equal(t, []any{int64(7)}, executed[0].Args)
}

func TestDelete_CompositeKey(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{RowsAffected: 1})

	err := Delete[testMembership](context.Background(), db, testMemberKey{OrgID: 1, UserID: 2})
	require.NoError(t, err)

	executed := fake.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t, `DELETE FROM "memberships" WHERE "org_id" = $1 AND "user_id" = $2`, executed[0].SQL)
	assert.Equal(t, []any{int64(1), int64(2)}, executed[0].Args)
}

func TestSave_ZeroScalarKey_InsertsNotUpdates(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "created_at"},
		Rows:    [][]driver.Value{testdb.Row(int64(1), time.Time{})},
	})

	row := &testAccount{Email: "a@b.com"}
	err := Save(context.Background(), db, row)
	require.NoError(t, err)

	executed := fake.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t,
		`INSERT INTO "accounts" ("email", "nickname") VALUES ($1, $2) RETURNING "id", "created_at"`,
		executed[0].SQL)
}

func TestSave_NonZeroScalarKey_UpdatesNotInserts(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{RowsAffected: 1})

	row := &testAccount{Model: Model[int64]{ID: 7}, Email: "a@b.com"}
	err := Save(context.Background(), db, row)
	require.NoError(t, err)

	executed := fake.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t,
		`UPDATE "accounts" SET "email" = $1, "created_at" = $2, "nickname" = $3 WHERE "id" = $4`,
		executed[0].SQL)
	assert.Equal(t, []any{"a@b.com", time.Time{}, nil, int64(7)}, executed[0].Args)
}

func TestSave_ZeroCompositeKey_RoutesToCreateAndFails(t *testing.T) {
	db, fake := newTestDB()

	row := &testMembership{Role: "admin"}
	err := Save(context.Background(), db, row)
	require.Error(t, err)
	var zErr *ErrZeroCompositeKey
	require.True(t, errors.As(err, &zErr))
	assert.Empty(t, fake.Executed())
}
