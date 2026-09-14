package testdb

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExec_CapturesSQLAndArgsExactly(t *testing.T) {
	db := New()
	sqlDB := db.Open()
	defer sqlDB.Close()

	db.Enqueue(Result{RowsAffected: 1})

	res, err := sqlDB.ExecContext(context.Background(), `UPDATE "users" SET "age" = $1 WHERE "id" = $2`, 30, 7)
	require.NoError(t, err)
	n, err := res.RowsAffected()
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)

	executed := db.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t, `UPDATE "users" SET "age" = $1 WHERE "id" = $2`, executed[0].SQL)
	assert.Equal(t, []any{30, 7}, executed[0].Args)
}

func TestQuery_CapturesSQLAndArgs_AndScansCannedRowsThroughRealDatabaseSQL(t *testing.T) {
	db := New()
	sqlDB := db.Open()
	defer sqlDB.Close()

	db.Enqueue(Result{
		Columns: []string{"id", "email"},
		Rows: [][]driver.Value{
			Row(int64(1), "a@b.com"),
			Row(int64(2), "c@d.com"),
		},
	})

	rows, err := sqlDB.QueryContext(context.Background(), `SELECT "id", "email" FROM "users" WHERE "active" = $1`, true)
	require.NoError(t, err)
	defer rows.Close()

	type row struct {
		id    int64
		email string
	}
	var got []row
	for rows.Next() {
		var r row
		require.NoError(t, rows.Scan(&r.id, &r.email))
		got = append(got, r)
	}
	require.NoError(t, rows.Err())

	assert.Equal(t, []row{{1, "a@b.com"}, {2, "c@d.com"}}, got)

	executed := db.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t, `SELECT "id", "email" FROM "users" WHERE "active" = $1`, executed[0].SQL)
	assert.Equal(t, []any{true}, executed[0].Args)
}

func TestFIFO_MultipleQueuedResults_ReplayInOrder(t *testing.T) {
	db := New()
	sqlDB := db.Open()
	defer sqlDB.Close()

	db.Enqueue(Result{RowsAffected: 1})
	db.Enqueue(Result{RowsAffected: 2})

	res1, err := sqlDB.ExecContext(context.Background(), "stmt one")
	require.NoError(t, err)
	n1, _ := res1.RowsAffected()
	assert.EqualValues(t, 1, n1)

	res2, err := sqlDB.ExecContext(context.Background(), "stmt two")
	require.NoError(t, err)
	n2, _ := res2.RowsAffected()
	assert.EqualValues(t, 2, n2)

	executed := db.Executed()
	require.Len(t, executed, 2)
	assert.Equal(t, "stmt one", executed[0].SQL)
	assert.Equal(t, "stmt two", executed[1].SQL)
}

func TestQueuedError_SurfacesFromExec(t *testing.T) {
	db := New()
	sqlDB := db.Open()
	defer sqlDB.Close()

	boom := errors.New("boom")
	db.Enqueue(Result{Err: boom})

	_, err := sqlDB.ExecContext(context.Background(), "stmt")
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
}

func TestQueuedError_SurfacesFromQuery(t *testing.T) {
	db := New()
	sqlDB := db.Open()
	defer sqlDB.Close()

	boom := errors.New("boom")
	db.Enqueue(Result{Err: boom})

	_, err := sqlDB.QueryContext(context.Background(), "stmt")
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
}

func TestNoQueuedResult_ReturnsClearErrorNotPanic(t *testing.T) {
	db := New()
	sqlDB := db.Open()
	defer sqlDB.Close()

	require.NotPanics(t, func() {
		_, err := sqlDB.ExecContext(context.Background(), "stmt")
		require.Error(t, err)
	})
}

func TestReset_ClearsExecutedHistoryAndQueue(t *testing.T) {
	db := New()
	sqlDB := db.Open()
	defer sqlDB.Close()

	db.Enqueue(Result{RowsAffected: 1})
	_, err := sqlDB.ExecContext(context.Background(), "stmt")
	require.NoError(t, err)
	require.Len(t, db.Executed(), 1)

	db.Reset()
	assert.Empty(t, db.Executed())

	_, err = sqlDB.ExecContext(context.Background(), "stmt")
	require.Error(t, err, "queue should be empty after Reset")
}

func TestPrepare_ReturnsUnsupportedError(t *testing.T) {
	db := New()
	sqlDB := db.Open()
	defer sqlDB.Close()

	conn, err := sqlDB.Conn(context.Background())
	require.NoError(t, err)
	defer conn.Close()

	_, err = sqlDB.PrepareContext(context.Background(), "stmt")
	if err == nil {
		t.Skip("this database/sql version did not need Prepare for this driver")
	}
	require.Error(t, err)
}

func TestRow_InvalidValue_PanicsAtSetupTime(t *testing.T) {
	assert.Panics(t, func() {
		Row(make(chan int))
	})
}

func TestBeginTx_CommitRecordsBeginAndCommitEvents(t *testing.T) {
	db := New()
	sqlDB := db.Open()
	defer sqlDB.Close()

	tx, err := sqlDB.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: true})
	require.NoError(t, err)

	db.Enqueue(Result{RowsAffected: 1})
	_, err = tx.ExecContext(context.Background(), "stmt")
	require.NoError(t, err)

	require.NoError(t, tx.Commit())

	events := db.TxEvents()
	require.Len(t, events, 2)
	assert.Equal(t, "BEGIN", events[0].Kind)
	assert.Equal(t, driver.IsolationLevel(sql.LevelSerializable), events[0].Isolation)
	assert.True(t, events[0].ReadOnly)
	assert.Equal(t, "COMMIT", events[1].Kind)

	require.Len(t, db.Executed(), 1)
}

func TestBeginTx_RollbackRecordsRollbackEvent(t *testing.T) {
	db := New()
	sqlDB := db.Open()
	defer sqlDB.Close()

	tx, err := sqlDB.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())

	events := db.TxEvents()
	require.Len(t, events, 2)
	assert.Equal(t, "BEGIN", events[0].Kind)
	assert.Equal(t, "ROLLBACK", events[1].Kind)
}

func TestRowsClosedCount_IncrementsOnClose(t *testing.T) {
	db := New()
	sqlDB := db.Open()
	defer sqlDB.Close()

	db.Enqueue(Result{Columns: []string{"id"}, Rows: [][]driver.Value{Row(int64(1))}})

	assert.Equal(t, 0, db.RowsClosedCount())
	rows, err := sqlDB.QueryContext(context.Background(), "stmt")
	require.NoError(t, err)
	require.NoError(t, rows.Close())
	assert.Equal(t, 1, db.RowsClosedCount())
}

func TestReset_ClearsTxEventsAndRowsClosedCount(t *testing.T) {
	db := New()
	sqlDB := db.Open()
	defer sqlDB.Close()

	tx, err := sqlDB.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	require.Len(t, db.TxEvents(), 2)

	db.Reset()
	assert.Empty(t, db.TxEvents())
	assert.Equal(t, 0, db.RowsClosedCount())
}

func TestArgs_PreserveExactGoTypes(t *testing.T) {
	db := New()
	sqlDB := db.Open()
	defer sqlDB.Close()

	db.Enqueue(Result{RowsAffected: 1})

	_, err := sqlDB.ExecContext(context.Background(), "stmt", 42)
	require.NoError(t, err)

	executed := db.Executed()
	require.Len(t, executed, 1)
	require.Len(t, executed[0].Args, 1)
	assert.IsType(t, int(0), executed[0].Args[0])
	assert.Equal(t, 42, executed[0].Args[0])
}
