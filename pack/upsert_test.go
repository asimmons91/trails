package pack

import (
	"context"
	"database/sql/driver"
	"testing"

	"github.com/asimmons91/trails/pack/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreate_OnConflictDoNothing_SkipsReturningEntirely(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{RowsAffected: 0})

	row := &testWidget{Count: 5}
	err := Create(context.Background(), db, row, OnConflict[testWidget](widgetCol.ID).DoNothing())
	require.NoError(t, err)

	executed := fake.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t,
		`INSERT INTO "widgets" ("count") VALUES ($1) ON CONFLICT ("id") DO NOTHING`,
		executed[0].SQL)
	assert.Equal(t, []any{uint64(5)}, executed[0].Args)
	// DoNothing skips RETURNING entirely, so the autoincrement ID is
	// left however it already was — zero here, never written to.
	assert.EqualValues(t, 0, row.ID)
}

func TestCreate_OnConflictDoUpdate_ComposesWithReturningWriteBack(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id"},
		Rows:    [][]driver.Value{testdb.Row(int64(7))},
	})

	row := &testWidget{Count: 5}
	err := Create(context.Background(), db, row,
		OnConflict[testWidget](widgetCol.ID).DoUpdate(widgetCol.Count.Set(5)))
	require.NoError(t, err)

	executed := fake.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t,
		`INSERT INTO "widgets" ("count") VALUES ($1) ON CONFLICT ("id") DO UPDATE SET "count" = $2 RETURNING "id"`,
		executed[0].SQL)
	assert.Equal(t, []any{uint64(5), uint64(5)}, executed[0].Args)
	// Unlike DoNothing, DO UPDATE guarantees one output row per input
	// row, so positional RETURNING write-back still runs.
	assert.EqualValues(t, 7, row.ID)
}

func TestCreate_OnConflictDoUpdate_SetExprReferencesExcluded(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id"},
		Rows:    [][]driver.Value{testdb.Row(int64(1))},
	})

	row := &testWidget{Count: 9}
	err := Create(context.Background(), db, row,
		OnConflict[testWidget](widgetCol.ID).DoUpdate(widgetCol.Count.SetExpr("EXCLUDED.count")))
	require.NoError(t, err)

	executed := fake.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t,
		`INSERT INTO "widgets" ("count") VALUES ($1) ON CONFLICT ("id") DO UPDATE SET "count" = EXCLUDED.count RETURNING "id"`,
		executed[0].SQL)
}

func TestCreate_NoConflictOption_RendersPlainInsert(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id"},
		Rows:    [][]driver.Value{testdb.Row(int64(1))},
	})

	row := &testWidget{Count: 1}
	err := Create(context.Background(), db, row)
	require.NoError(t, err)

	executed := fake.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t, `INSERT INTO "widgets" ("count") VALUES ($1) RETURNING "id"`, executed[0].SQL)
}
