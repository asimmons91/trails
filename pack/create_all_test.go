package pack

import (
	"context"
	"database/sql/driver"
	"testing"
	"time"

	"github.com/asimmons91/trails/pack/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateAll_MultiRowInsert_WritesReturningValuesBackPositionally(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id"},
		Rows: [][]driver.Value{
			testdb.Row(int64(1)),
			testdb.Row(int64(2)),
			testdb.Row(int64(3)),
		},
	})

	rows := []*testWidget{{Count: 10}, {Count: 20}, {Count: 30}}
	err := CreateAll(context.Background(), db, rows)
	require.NoError(t, err)

	executed := fake.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t,
		`INSERT INTO "widgets" ("count") VALUES ($1), ($2), ($3) RETURNING "id"`,
		executed[0].SQL)
	assert.Equal(t, []any{uint64(10), uint64(20), uint64(30)}, executed[0].Args)

	assert.EqualValues(t, 1, rows[0].ID)
	assert.EqualValues(t, 2, rows[1].ID)
	assert.EqualValues(t, 3, rows[2].ID)
}

func TestCreateAll_NeverOmitsDefaultColumns_EvenWhenZeroOnSomeRows(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id"},
		Rows:    [][]driver.Value{testdb.Row(int64(1)), testdb.Row(int64(2))},
	})

	// Row 0 has a zero CreatedAt; row 1 has an explicit one. CreateAll must
	// still send both explicitly — never omit default:-tagged columns.
	explicit := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := []*testAccount{
		{Email: "a@b.com"},
		{Email: "c@d.com", CreatedAt: explicit},
	}
	err := CreateAll(context.Background(), db, rows)
	require.NoError(t, err)

	executed := fake.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t,
		`INSERT INTO "accounts" ("email", "created_at", "nickname") VALUES ($1, $2, $3), ($4, $5, $6) RETURNING "id"`,
		executed[0].SQL)
	assert.Equal(t, []any{"a@b.com", time.Time{}, nil, "c@d.com", explicit, nil}, executed[0].Args)
}

func TestCreateAll_ZeroRows_IsNoOp(t *testing.T) {
	db, fake := newTestDB()
	err := CreateAll[testWidget](context.Background(), db, nil)
	require.NoError(t, err)
	assert.Empty(t, fake.Executed())
}

func TestCreateAll_CompositeKey_ZeroKeyOnAnyRow_AbortsBeforeAnySQL(t *testing.T) {
	db, fake := newTestDB()

	rows := []*testMembership{
		{Model: Model[testMemberKey]{ID: testMemberKey{OrgID: 1, UserID: 2}}, Role: "admin"},
		{Role: "member"}, // zero key
	}
	err := CreateAll(context.Background(), db, rows)
	require.Error(t, err)
	var zErr *ErrZeroCompositeKey
	require.ErrorAs(t, err, &zErr)
	assert.Empty(t, fake.Executed())
}

func TestCreateAll_BatchesAtConfiguredSize(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"id"}, Rows: [][]driver.Value{testdb.Row(int64(1)), testdb.Row(int64(2))}})
	fake.Enqueue(testdb.Result{Columns: []string{"id"}, Rows: [][]driver.Value{testdb.Row(int64(3))}})

	rows := []*testWidget{{Count: 1}, {Count: 2}, {Count: 3}}
	err := CreateAll(context.Background(), db, rows, WithBatchSize(2))
	require.NoError(t, err)

	executed := fake.Executed()
	require.Len(t, executed, 2)
	assert.Equal(t, `INSERT INTO "widgets" ("count") VALUES ($1), ($2) RETURNING "id"`, executed[0].SQL)
	assert.Equal(t, `INSERT INTO "widgets" ("count") VALUES ($1) RETURNING "id"`, executed[1].SQL)
	assert.EqualValues(t, 1, rows[0].ID)
	assert.EqualValues(t, 2, rows[1].ID)
	assert.EqualValues(t, 3, rows[2].ID)
}

func TestCreateAll_BeforeInsertHook_FiresPerRow_InOrder_BeforeAnySQL(t *testing.T) {
	resetHookLog()
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id"},
		Rows:    [][]driver.Value{testdb.Row(int64(1)), testdb.Row(int64(2))},
	})

	rows := []*testHookedItem{{Name: "a"}, {Name: "b"}}
	err := CreateAll(context.Background(), db, rows)
	require.NoError(t, err)

	// BeforeInsert x2, then AfterInsert x2 (chunk completes as one statement).
	require.Len(t, hookLog, 4)
	assert.Equal(t, "BeforeInsert", hookLog[0].Op)
	assert.Equal(t, "BeforeInsert", hookLog[1].Op)
	assert.Equal(t, "AfterInsert", hookLog[2].Op)
	assert.Equal(t, "AfterInsert", hookLog[3].Op)
}

func TestCreateAll_BeforeInsertHookErrorOnAnyRow_AbortsWholeBatch(t *testing.T) {
	resetHookLog()
	db, fake := newTestDB()

	rows := []*testHookedItem{{Name: "a"}, {Name: "b", FailHook: "BeforeInsert"}, {Name: "c"}}
	err := CreateAll(context.Background(), db, rows)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "BeforeInsert hook on testHookedItem")
	assert.Empty(t, fake.Executed())
	// Row c's hook never ran — the batch aborted at row b.
	require.Len(t, hookLog, 2)
	assert.Equal(t, "BeforeInsert", hookLog[0].Op)
	assert.Equal(t, "BeforeInsert", hookLog[1].Op)
}

func TestCreateAll_OnConflictDoNothing_SkipsReturningEntirely(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{RowsAffected: 1})

	rows := []*testWidget{{Count: 1}, {Count: 2}}
	err := CreateAll(context.Background(), db, rows,
		OnConflict[testWidget](widgetCol.ID).DoNothing())
	require.NoError(t, err)

	executed := fake.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t,
		`INSERT INTO "widgets" ("count") VALUES ($1), ($2) ON CONFLICT ("id") DO NOTHING`,
		executed[0].SQL)
}
