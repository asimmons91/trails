package pack

import (
	"context"
	"database/sql/driver"
	"errors"
	"reflect"
	"testing"

	"github.com/asimmons91/trails/pack/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type valueReceiverInserter struct {
	Model[int64] `db:"table:value_receiver_inserters"`
}

func (valueReceiverInserter) BeforeInsert(ctx context.Context) error { return nil }

func TestField_ValueReceiverHookMethod_PanicsAtSchemaBuildTime(t *testing.T) {
	assert.Panics(t, func() {
		Field(func(m *valueReceiverInserter) *int64 { return &m.ID })
	})
}

func TestCreate_BeforeInsertHook_RunsBeforeSQLConstruction(t *testing.T) {
	resetHookLog()
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id"},
		Rows:    [][]driver.Value{testdb.Row(int64(1))},
	})

	row := &testHookedItem{Name: "widget"}
	err := Create(context.Background(), db, row)
	require.NoError(t, err)

	require.Len(t, hookLog, 2)
	assert.Equal(t, "BeforeInsert", hookLog[0].Op)
	assert.Equal(t, "AfterInsert", hookLog[1].Op)
	// AfterInsert observed the RETURNING-populated ID (R8.30).
	assert.EqualValues(t, 1, hookLog[1].ID)
}

func TestCreate_BeforeInsertHookError_AbortsBeforeAnySQL(t *testing.T) {
	resetHookLog()
	db, fake := newTestDB()

	row := &testHookedItem{Name: "widget", FailHook: "BeforeInsert"}
	err := Create(context.Background(), db, row)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "BeforeInsert hook on testHookedItem")
	assert.Empty(t, fake.Executed())
}

func TestCreate_AfterInsertHookError_Propagates(t *testing.T) {
	resetHookLog()
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id"},
		Rows:    [][]driver.Value{testdb.Row(int64(1))},
	})

	row := &testHookedItem{Name: "widget", FailHook: "AfterInsert"}
	err := Create(context.Background(), db, row)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AfterInsert hook on testHookedItem")
	require.Len(t, fake.Executed(), 1) // the INSERT itself did run
}

func TestUpdate_BeforeAfterUpdateHooks_FireInOrder(t *testing.T) {
	resetHookLog()
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{RowsAffected: 1})

	row := &testHookedItem{Model: Model[int64]{ID: 1}, Name: "widget"}
	err := Update(context.Background(), db, row)
	require.NoError(t, err)

	require.Len(t, hookLog, 2)
	assert.Equal(t, "BeforeUpdate", hookLog[0].Op)
	assert.Equal(t, "AfterUpdate", hookLog[1].Op)
	require.Len(t, fake.Executed(), 1)
}

func TestUpdate_BeforeUpdateHookError_AbortsBeforeAnySQL(t *testing.T) {
	resetHookLog()
	db, fake := newTestDB()

	row := &testHookedItem{Model: Model[int64]{ID: 1}, Name: "widget", FailHook: "BeforeUpdate"}
	err := Update(context.Background(), db, row)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "BeforeUpdate hook on testHookedItem")
	assert.Empty(t, fake.Executed())
}

func TestDelete_HookedModel_FetchesRowFirst_ThenFiresHooksInOrder(t *testing.T) {
	resetHookLog()
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{ // the fetch (ByID) inside Delete
		Columns: []string{"id", "name"},
		Rows:    [][]driver.Value{testdb.Row(int64(1), "widget")},
	})
	fake.Enqueue(testdb.Result{RowsAffected: 1}) // the DELETE itself

	err := Delete[testHookedItem](context.Background(), db, int64(1))
	require.NoError(t, err)

	require.Len(t, fake.Executed(), 2)
	// testHookedItem also implements AfterScanner, so the fetch itself
	// (a real Find/First round trip) legitimately fires AfterScan first —
	// this is correct, not a leak: the fetch is a genuine read.
	require.Len(t, hookLog, 3)
	assert.Equal(t, "AfterScan", hookLog[0].Op)
	assert.Equal(t, "BeforeDelete", hookLog[1].Op)
	assert.Equal(t, "AfterDelete", hookLog[2].Op)
	assert.EqualValues(t, 1, hookLog[1].ID)
}

func TestDelete_HookedModel_RowNotFound_ReturnsNilWithoutFiringHooks(t *testing.T) {
	resetHookLog()
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"id", "name"}}) // ByID finds nothing

	err := Delete[testHookedItem](context.Background(), db, int64(1))
	require.NoError(t, err)
	assert.Empty(t, hookLog)
	require.Len(t, fake.Executed(), 1) // only the fetch, no DELETE issued
}

func TestDelete_UnhookedModel_NeverFetchesFirst(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{RowsAffected: 1})

	err := Delete[testAccount](context.Background(), db, int64(1))
	require.NoError(t, err)
	require.Len(t, fake.Executed(), 1) // no fetch — straight to DELETE
	assert.Equal(t, `DELETE FROM "accounts" WHERE "id" = $1`, fake.Executed()[0].SQL)
}

func TestFind_AfterScanHook_FiresOncePerRow(t *testing.T) {
	resetHookLog()
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "name"},
		Rows: [][]driver.Value{
			testdb.Row(int64(1), "a"),
			testdb.Row(int64(2), "b"),
		},
	})

	got, err := Of[testHookedItem](db).Find(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 2)

	require.Len(t, hookLog, 2)
	assert.Equal(t, "AfterScan", hookLog[0].Op)
	assert.EqualValues(t, 1, hookLog[0].ID)
	assert.Equal(t, "AfterScan", hookLog[1].Op)
	assert.EqualValues(t, 2, hookLog[1].ID)
}

func TestFind_UnhookedModel_NoHookCallsAtAll(t *testing.T) {
	resetHookLog()
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "email", "age"},
		Rows:    [][]driver.Value{testdb.Row(int64(1), "a@b.com", int64(30))},
	})

	_, err := Of[testUser](db).Find(context.Background())
	require.NoError(t, err)
	assert.Empty(t, hookLog)
}

func TestQueryWriteUpdate_BlockedByHooks_ThenSkipHooksUnblocks(t *testing.T) {
	resetHookLog()
	db, fake := newTestDB()

	_, err := Of[testHookedItem](db).
		Where(hookedItemCol.ID.Eq(int64(1))).
		Update(context.Background(), hookedItemCol.Name.Set("new"))
	require.Error(t, err)
	var blocked *ErrSetOperationBlockedByHooks
	require.True(t, errors.As(err, &blocked))
	assert.Equal(t, "testHookedItem", blocked.Model)
	assert.Equal(t, "Update", blocked.Operation)
	assert.Empty(t, fake.Executed())
	assert.Empty(t, hookLog, "no hook can fire for a set-based operation — there is no Go instance")

	fake.Enqueue(testdb.Result{RowsAffected: 1})
	n, err := Of[testHookedItem](db).
		Where(hookedItemCol.ID.Eq(int64(1))).
		SkipHooks().
		Update(context.Background(), hookedItemCol.Name.Set("new"))
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)
	require.Len(t, fake.Executed(), 1)
	assert.Equal(t, `UPDATE "hooked_items" AS "hooked_items" SET "name" = $1 WHERE "hooked_items"."id" = $2`, fake.Executed()[0].SQL)
}

func TestQueryWriteDelete_BlockedByHooks(t *testing.T) {
	db, fake := newTestDB()

	_, err := Of[testHookedItem](db).
		Where(hookedItemCol.ID.Eq(int64(1))).
		Delete(context.Background())
	require.Error(t, err)
	var blocked *ErrSetOperationBlockedByHooks
	require.True(t, errors.As(err, &blocked))
	assert.Equal(t, "Delete", blocked.Operation)
	assert.Empty(t, fake.Executed())
}

func TestFixture_HookedItem_ImplementsAllSevenHooks(t *testing.T) {
	pt := reflect.TypeFor[*testHookedItem]()
	for _, iface := range []reflect.Type{
		reflect.TypeFor[BeforeInserter](),
		reflect.TypeFor[AfterInserter](),
		reflect.TypeFor[BeforeUpdater](),
		reflect.TypeFor[AfterUpdater](),
		reflect.TypeFor[BeforeDeleter](),
		reflect.TypeFor[AfterDeleter](),
		reflect.TypeFor[AfterScanner](),
	} {
		assert.True(t, pt.Implements(iface), "expected *testHookedItem to implement %s", iface)
	}
}
