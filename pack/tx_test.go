package pack

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"

	"github.com/asimmons91/trails/pack/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTx_CommitsOnNilReturn(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{RowsAffected: 1})

	err := db.Tx(context.Background(), func(tx *DB) error {
		_, err := Of[testAccount](tx).Where(accountCol.ID.Eq(int64(1))).Delete(context.Background())
		return err
	})
	require.NoError(t, err)

	events := fake.TxEvents()
	require.Len(t, events, 2)
	assert.Equal(t, "BEGIN", events[0].Kind)
	assert.Equal(t, "COMMIT", events[1].Kind)
}

func TestTx_RollsBackOnError(t *testing.T) {
	db, fake := newTestDB()
	boom := errors.New("boom")

	err := db.Tx(context.Background(), func(tx *DB) error {
		return boom
	})
	require.ErrorIs(t, err, boom)

	events := fake.TxEvents()
	require.Len(t, events, 2)
	assert.Equal(t, "BEGIN", events[0].Kind)
	assert.Equal(t, "ROLLBACK", events[1].Kind)
}

func TestTx_RollsBackAndRePanicsOnPanic(t *testing.T) {
	db, fake := newTestDB()

	assert.Panics(t, func() {
		_ = db.Tx(context.Background(), func(tx *DB) error {
			panic("boom")
		})
	})

	events := fake.TxEvents()
	require.Len(t, events, 2)
	assert.Equal(t, "BEGIN", events[0].Kind)
	assert.Equal(t, "ROLLBACK", events[1].Kind)
}

func TestTx_NestedTx_JoinsRatherThanOpeningASecond(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{RowsAffected: 1})

	err := db.Tx(context.Background(), func(outer *DB) error {
		return outer.Tx(context.Background(), func(inner *DB) error {
			_, err := Of[testAccount](inner).Where(accountCol.ID.Eq(int64(1))).Delete(context.Background())
			return err
		})
	})
	require.NoError(t, err)

	events := fake.TxEvents()
	require.Len(t, events, 2, "nested Tx must join, not open a second BEGIN/COMMIT pair")
	assert.Equal(t, "BEGIN", events[0].Kind)
	assert.Equal(t, "COMMIT", events[1].Kind)
}

func TestTx_NestedTx_InnerErrorPropagatesToOuterRollback(t *testing.T) {
	db, fake := newTestDB()
	boom := errors.New("boom")

	err := db.Tx(context.Background(), func(outer *DB) error {
		return outer.Tx(context.Background(), func(inner *DB) error {
			return boom
		})
	})
	require.ErrorIs(t, err, boom)

	events := fake.TxEvents()
	require.Len(t, events, 2)
	assert.Equal(t, "ROLLBACK", events[1].Kind)
}

func TestBeginTx_ManualCommit(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{RowsAffected: 1})

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	_, err = Of[testAccount](tx).Where(accountCol.ID.Eq(int64(1))).Delete(context.Background())
	require.NoError(t, err)

	require.NoError(t, tx.Commit())

	events := fake.TxEvents()
	require.Len(t, events, 2)
	assert.Equal(t, "BEGIN", events[0].Kind)
	assert.Equal(t, "COMMIT", events[1].Kind)
}

func TestBeginTx_ManualRollback(t *testing.T) {
	db, fake := newTestDB()

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())

	events := fake.TxEvents()
	require.Len(t, events, 2)
	assert.Equal(t, "ROLLBACK", events[1].Kind)
}

func TestBeginTx_OnAlreadyInTxDB_ReturnsError(t *testing.T) {
	db, _ := newTestDB()

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	_, err = tx.BeginTx(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already inside a transaction")
}

func TestCommit_OnNonTxDB_ReturnsError(t *testing.T) {
	db, _ := newTestDB()
	err := db.Commit()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a transaction")
}

func TestRollback_OnNonTxDB_ReturnsError(t *testing.T) {
	db, _ := newTestDB()
	err := db.Rollback()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a transaction")
}

func TestBeginTx_PassesTxOptionsThroughToDriver(t *testing.T) {
	db, fake := newTestDB()

	tx, err := db.BeginTx(context.Background(), &sql.TxOptions{
		Isolation: sql.LevelSerializable,
		ReadOnly:  true,
	})
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())

	events := fake.TxEvents()
	require.Len(t, events, 2)
	assert.Equal(t, driver.IsolationLevel(sql.LevelSerializable), events[0].Isolation)
	assert.True(t, events[0].ReadOnly)
}

func TestBeginTx_ReturnedDBWorksAsAnOrdinaryDB(t *testing.T) {
	// R10.4: the *DB handed back is usable everywhere a *DB is accepted.
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "email", "created_at", "nickname"},
	})

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	_, err = Of[testAccount](tx).Find(context.Background())
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())

	require.Len(t, fake.Executed(), 1)
}
