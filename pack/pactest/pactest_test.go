package pactest_test

import (
	"context"
	"testing"

	"github.com/asimmons91/trails/pack"
	"github.com/asimmons91/trails/pack/dialect/sqlitedialect"
	"github.com/asimmons91/trails/pack/internal/testdb"
	"github.com/asimmons91/trails/pack/pactest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTxDB_BeginsAndRollsBackOnCleanup(t *testing.T) {
	fake := testdb.New()
	db := pack.Open(fake.Open(), sqlitedialect.New())

	t.Run("subtest", func(t *testing.T) {
		txDB := pactest.TxDB(t, context.Background(), db)
		assert.True(t, txDB.InTransaction())
	})

	events := fake.TxEvents()
	require.Len(t, events, 2)
	assert.Equal(t, "BEGIN", events[0].Kind)
	assert.Equal(t, "ROLLBACK", events[1].Kind)
}

func TestTxDB_NestedTxNestsInsteadOfCommittingForReal(t *testing.T) {
	fake := testdb.New()
	db := pack.Open(fake.Open(), sqlitedialect.New())
	fake.Enqueue(testdb.Result{}) // SAVEPOINT
	fake.Enqueue(testdb.Result{}) // RELEASE SAVEPOINT

	t.Run("subtest", func(t *testing.T) {
		txDB := pactest.TxDB(t, context.Background(), db)

		err := txDB.Tx(context.Background(), func(nested *pack.DB) error {
			assert.True(t, nested.InTransaction())
			return nil
		})
		require.NoError(t, err)
	})

	// Only the outer BeginTx/Rollback show up as real transaction events;
	// the nested db.Tx used a SAVEPOINT (a plain Exec) instead of a second
	// BEGIN/COMMIT, so it never committed anything for real.
	events := fake.TxEvents()
	require.Len(t, events, 2)
	assert.Equal(t, "BEGIN", events[0].Kind)
	assert.Equal(t, "ROLLBACK", events[1].Kind)

	var sawSavepoint, sawRelease bool
	for _, rec := range fake.Executed() {
		if rec.SQL == "SAVEPOINT pack_sp1" {
			sawSavepoint = true
		}
		if rec.SQL == "RELEASE SAVEPOINT pack_sp1" {
			sawRelease = true
		}
	}
	assert.True(t, sawSavepoint, "expected a SAVEPOINT to be issued")
	assert.True(t, sawRelease, "expected the savepoint to be released, not rolled back")
}
