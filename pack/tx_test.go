package pack

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"

	"github.com/asimmons91/trails/pack/dialect/pgdialect"
	"github.com/asimmons91/trails/pack/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// noSavepointDialect mimics a dialect that lacks SAVEPOINT support, to
// exercise BeginTx's nesting-rejection path.
type noSavepointDialect struct{ pgdialect.Postgres }

func (noSavepointDialect) SupportsSavepoints() bool { return false }

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

func TestTx_NestedTx_UsesSavepointNotSecondBegin(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{})                // SAVEPOINT
	fake.Enqueue(testdb.Result{RowsAffected: 1}) // DELETE
	fake.Enqueue(testdb.Result{})                // RELEASE SAVEPOINT

	err := db.Tx(context.Background(), func(outer *DB) error {
		return outer.Tx(context.Background(), func(inner *DB) error {
			_, err := Of[testAccount](inner).Where(accountCol.ID.Eq(int64(1))).Delete(context.Background())
			return err
		})
	})
	require.NoError(t, err)

	events := fake.TxEvents()
	require.Len(t, events, 2, "nested Tx must use a SAVEPOINT, not open a second BEGIN/COMMIT pair")
	assert.Equal(t, "BEGIN", events[0].Kind)
	assert.Equal(t, "COMMIT", events[1].Kind)

	executed := fake.Executed()
	require.Len(t, executed, 3)
	assert.Equal(t, "SAVEPOINT pack_sp1", executed[0].SQL)
	assert.Contains(t, executed[1].SQL, "DELETE")
	assert.Equal(t, "RELEASE SAVEPOINT pack_sp1", executed[2].SQL)
}

func TestTx_NestedTx_InnerErrorPropagatesToOuterRollback(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{}) // SAVEPOINT
	fake.Enqueue(testdb.Result{}) // ROLLBACK TO SAVEPOINT
	boom := errors.New("boom")

	err := db.Tx(context.Background(), func(outer *DB) error {
		return outer.Tx(context.Background(), func(inner *DB) error {
			return boom
		})
	})
	require.ErrorIs(t, err, boom)

	executed := fake.Executed()
	require.Len(t, executed, 2)
	assert.Equal(t, "SAVEPOINT pack_sp1", executed[0].SQL)
	assert.Equal(t, "ROLLBACK TO SAVEPOINT pack_sp1", executed[1].SQL)

	events := fake.TxEvents()
	require.Len(t, events, 2, "the propagated inner error must still roll back the real transaction")
	assert.Equal(t, "ROLLBACK", events[1].Kind)
}

func TestTx_NestedTx_SwallowedInnerErrorOnlyRollsBackSavepoint(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{}) // SAVEPOINT
	fake.Enqueue(testdb.Result{}) // ROLLBACK TO SAVEPOINT
	boom := errors.New("boom")

	err := db.Tx(context.Background(), func(outer *DB) error {
		innerErr := outer.Tx(context.Background(), func(inner *DB) error {
			return boom
		})
		require.ErrorIs(t, innerErr, boom)
		return nil // swallow it: only the inner block should be undone
	})
	require.NoError(t, err)

	executed := fake.Executed()
	require.Len(t, executed, 2)
	assert.Equal(t, "SAVEPOINT pack_sp1", executed[0].SQL)
	assert.Equal(t, "ROLLBACK TO SAVEPOINT pack_sp1", executed[1].SQL)

	events := fake.TxEvents()
	require.Len(t, events, 2, "swallowing the inner error must let the outer transaction commit")
	assert.Equal(t, "BEGIN", events[0].Kind)
	assert.Equal(t, "COMMIT", events[1].Kind)
}

func TestTx_TwoLevelsOfNesting_UseDistinctSavepoints(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{}) // SAVEPOINT pack_sp1
	fake.Enqueue(testdb.Result{}) // SAVEPOINT pack_sp2
	fake.Enqueue(testdb.Result{}) // RELEASE SAVEPOINT pack_sp2
	fake.Enqueue(testdb.Result{}) // RELEASE SAVEPOINT pack_sp1

	err := db.Tx(context.Background(), func(l1 *DB) error {
		return l1.Tx(context.Background(), func(l2 *DB) error {
			return l2.Tx(context.Background(), func(l3 *DB) error {
				return nil
			})
		})
	})
	require.NoError(t, err)

	executed := fake.Executed()
	require.Len(t, executed, 4)
	assert.Equal(t, "SAVEPOINT pack_sp1", executed[0].SQL)
	assert.Equal(t, "SAVEPOINT pack_sp2", executed[1].SQL)
	assert.Equal(t, "RELEASE SAVEPOINT pack_sp2", executed[2].SQL)
	assert.Equal(t, "RELEASE SAVEPOINT pack_sp1", executed[3].SQL)
}

func TestBeginTx_OnDialectWithoutSavepointSupport_ReturnsError(t *testing.T) {
	fake := testdb.New()
	db := Open(fake.Open(), noSavepointDialect{})

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	_, err = tx.BeginTx(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not support savepoints")
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

func TestBeginTx_OnAlreadyInTxDB_UsesSavepoint(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{}) // SAVEPOINT
	fake.Enqueue(testdb.Result{}) // ROLLBACK TO SAVEPOINT

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	nested, err := tx.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	executed := fake.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t, "SAVEPOINT pack_sp1", executed[0].SQL)

	require.NoError(t, nested.Rollback())
	require.NoError(t, tx.Rollback())
}

func TestBeginTx_NestedCommit_ReleasesSavepoint(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{}) // SAVEPOINT
	fake.Enqueue(testdb.Result{}) // RELEASE SAVEPOINT

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	nested, err := tx.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	require.NoError(t, nested.Commit())
	require.NoError(t, tx.Rollback())

	executed := fake.Executed()
	require.Len(t, executed, 2)
	assert.Equal(t, "SAVEPOINT pack_sp1", executed[0].SQL)
	assert.Equal(t, "RELEASE SAVEPOINT pack_sp1", executed[1].SQL)
}

func TestBeginTx_NestedRollback_RollsBackToSavepoint(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{}) // SAVEPOINT
	fake.Enqueue(testdb.Result{}) // ROLLBACK TO SAVEPOINT

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	nested, err := tx.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	require.NoError(t, nested.Rollback())
	require.NoError(t, tx.Commit())

	executed := fake.Executed()
	require.Len(t, executed, 2)
	assert.Equal(t, "SAVEPOINT pack_sp1", executed[0].SQL)
	assert.Equal(t, "ROLLBACK TO SAVEPOINT pack_sp1", executed[1].SQL)
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
