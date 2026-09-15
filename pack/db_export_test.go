package pack

import (
	"context"
	"testing"

	"github.com/asimmons91/trails/pack/dialect/pgdialect"
	"github.com/asimmons91/trails/pack/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDB_Dialect_ReturnsConfiguredDialect(t *testing.T) {
	db, _ := newTestDB()
	assert.Equal(t, "postgres", db.Dialect().Name())
}

func TestDB_ExecContext_FiresQueryHooksLikeInternalOperations(t *testing.T) {
	fake := testdb.New()
	hook := &recordingQueryHook{}
	db := Open(fake.Open(), pgdialect.New(), WithQueryHook(hook))
	fake.Enqueue(testdb.Result{RowsAffected: 1})

	_, err := db.ExecContext(context.Background(), "CreateTable", "Widget", `CREATE TABLE "widgets" ("id" BIGINT)`, nil)
	require.NoError(t, err)

	require.Len(t, hook.before, 1)
	require.Len(t, hook.after, 1)
	assert.Equal(t, "CreateTable", hook.before[0].Operation)
	assert.Equal(t, "Widget", hook.before[0].Model)
	assert.Equal(t, `CREATE TABLE "widgets" ("id" BIGINT)`, hook.before[0].SQL)
	assert.NoError(t, hook.after[0].Err)

	require.Len(t, fake.Executed(), 1)
	assert.Equal(t, `CREATE TABLE "widgets" ("id" BIGINT)`, fake.Executed()[0].SQL)
}

func TestDB_QueryContext_FiresQueryHooksLikeInternalOperations(t *testing.T) {
	fake := testdb.New()
	hook := &recordingQueryHook{}
	db := Open(fake.Open(), pgdialect.New(), WithQueryHook(hook))
	fake.Enqueue(testdb.Result{Columns: []string{"exists"}})

	rows, err := db.QueryContext(context.Background(), "HasTable", "Widget", "SELECT EXISTS (...)", nil)
	require.NoError(t, err)
	require.NoError(t, rows.Close())

	require.Len(t, hook.before, 1)
	assert.Equal(t, "HasTable", hook.before[0].Operation)
	assert.Equal(t, "Widget", hook.before[0].Model)
}

func TestDB_InTransaction_FalseOnRootDB(t *testing.T) {
	db, _ := newTestDB()
	assert.False(t, db.InTransaction())
}

func TestDB_InTransaction_TrueInsideTx(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{})

	err := db.Tx(context.Background(), func(tx *DB) error {
		assert.True(t, tx.InTransaction())
		return nil
	})
	require.NoError(t, err)
}

func TestDB_PinnedConn_RunsCallbackAgainstAWorkingHandle(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{RowsAffected: 1})

	var ran bool
	err := db.PinnedConn(context.Background(), func(pinned *DB) error {
		ran = true
		_, err := pinned.ExecContext(context.Background(), "Raw", "", "PRAGMA foreign_keys = OFF", nil)
		return err
	})
	require.NoError(t, err)
	assert.True(t, ran)
	require.Len(t, fake.Executed(), 1)
	assert.Equal(t, "PRAGMA foreign_keys = OFF", fake.Executed()[0].SQL)
}

func TestDB_PinnedConn_SupportsNestedTx(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{})                // BEGIN's first statement inside the tx
	fake.Enqueue(testdb.Result{RowsAffected: 1}) // the statement itself

	err := db.PinnedConn(context.Background(), func(pinned *DB) error {
		return pinned.Tx(context.Background(), func(tx *DB) error {
			_, err := tx.ExecContext(context.Background(), "Raw", "", "CREATE TABLE widgets (id INTEGER)", nil)
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(context.Background(), "Raw", "", "INSERT INTO widgets VALUES (1)", nil)
			return err
		})
	})
	require.NoError(t, err)

	events := fake.TxEvents()
	require.Len(t, events, 2)
	assert.Equal(t, "BEGIN", events[0].Kind)
	assert.Equal(t, "COMMIT", events[1].Kind)
}

func TestDB_PinnedConn_ReturnsErrPinnedConnUnsupported_WhenAlreadyInTransaction(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{})

	err := db.Tx(context.Background(), func(tx *DB) error {
		return tx.PinnedConn(context.Background(), func(*DB) error {
			t.Fatal("callback must not run: a transaction handle cannot pin a connection")
			return nil
		})
	})
	require.ErrorIs(t, err, ErrPinnedConnUnsupported)
}

func TestDB_PinnedConn_ReleasesConnectionOnReturn(t *testing.T) {
	db, _ := newTestDB()
	boom := context.Canceled

	err := db.PinnedConn(context.Background(), func(*DB) error {
		return boom
	})
	require.ErrorIs(t, err, boom)
}
