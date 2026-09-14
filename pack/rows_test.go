package pack

import (
	"context"
	"database/sql/driver"
	"testing"

	"github.com/asimmons91/trails/pack/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRows_RendersGoldenSQL_AndYieldsEveryRow(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "name"},
		Rows: [][]driver.Value{
			testdb.Row(int64(1), "a"),
			testdb.Row(int64(2), "b"),
		},
	})

	var got []testHookedItem
	var errs []error
	for row, err := range Of[testHookedItem](db).Rows(context.Background()) {
		got = append(got, row)
		errs = append(errs, err)
	}
	require.Len(t, got, 2)
	for _, e := range errs {
		require.NoError(t, e)
	}
	assert.EqualValues(t, 1, got[0].ID)
	assert.EqualValues(t, 2, got[1].ID)

	executed := fake.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t,
		`SELECT "hooked_items"."id", "hooked_items"."name" FROM "hooked_items" AS "hooked_items"`,
		executed[0].SQL)
}

func TestRows_FiresAfterScanPerRow(t *testing.T) {
	resetHookLog()
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "name"},
		Rows: [][]driver.Value{
			testdb.Row(int64(1), "a"),
			testdb.Row(int64(2), "b"),
		},
	})

	for range Of[testHookedItem](db).Rows(context.Background()) {
	}

	require.Len(t, hookLog, 2)
	assert.Equal(t, "AfterScan", hookLog[0].Op)
	assert.Equal(t, "AfterScan", hookLog[1].Op)
}

func TestRows_EarlyBreak_ClosesUnderlyingRows(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "name"},
		Rows: [][]driver.Value{
			testdb.Row(int64(1), "a"),
			testdb.Row(int64(2), "b"),
			testdb.Row(int64(3), "c"),
		},
	})

	count := 0
	for range Of[testHookedItem](db).Rows(context.Background()) {
		count++
		if count == 1 {
			break
		}
	}
	assert.Equal(t, 1, count)
	assert.Equal(t, 1, fake.RowsClosedCount())
}

func TestRows_PanicInLoopBody_StillClosesUnderlyingRows(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "name"},
		Rows: [][]driver.Value{
			testdb.Row(int64(1), "a"),
		},
	})

	assert.Panics(t, func() {
		for range Of[testHookedItem](db).Rows(context.Background()) {
			panic("boom")
		}
	})
	assert.Equal(t, 1, fake.RowsClosedCount())
}

func TestRows_ScanErrorMidStream_StopsIterationAfterYieldingIt(t *testing.T) {
	db, fake := newTestDB()
	// Row 2's "id" column is a string where an int64 is expected — Scan fails.
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "name"},
		Rows: [][]driver.Value{
			testdb.Row(int64(1), "a"),
			testdb.Row("not-an-int", "b"),
		},
	})

	var results []int
	var lastErr error
	for _, err := range Of[testHookedItem](db).Rows(context.Background()) {
		results = append(results, 1)
		if err != nil {
			lastErr = err
			break
		}
	}
	require.Len(t, results, 2)
	require.Error(t, lastErr)
}

func TestRows_CancelledContext_YieldsCtxErrAndStops(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "name"},
		Rows: [][]driver.Value{
			testdb.Row(int64(1), "a"),
			testdb.Row(int64(2), "b"),
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var gotErr error
	n := 0
	for _, err := range Of[testHookedItem](db).Rows(ctx) {
		n++
		if err != nil {
			gotErr = err
		}
	}
	assert.Equal(t, 1, n)
	require.Error(t, gotErr)
	assert.ErrorIs(t, gotErr, context.Canceled)
}

func TestRows_PreloadCombined_ReturnsConflictErrorBeforeAnySQL(t *testing.T) {
	db, fake := newTestDB()

	n := 0
	var gotErr error
	for _, err := range Of[testAccount](db).Preload(accountRel.Posts).Rows(context.Background()) {
		n++
		gotErr = err
	}
	assert.Equal(t, 1, n)
	require.Error(t, gotErr)
	assert.Contains(t, gotErr.Error(), "cannot be combined with Preload")
	assert.Empty(t, fake.Executed())
}
