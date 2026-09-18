// Package pactest provides a *pack.DB test helper: TxDB wraps db in a
// transaction that's automatically rolled back when the test ends, so
// tests can write freely without polluting shared state.
package pactest

import (
	"context"
	"testing"

	"github.com/asimmons91/trails/pack"
)

// TxDB begins a transaction on db and registers a t.Cleanup to roll it
// back, returning the transaction-scoped *pack.DB for the test to use. It
// calls t.Fatalf if BeginTx fails.
func TxDB(t testing.TB, ctx context.Context, db *pack.DB) *pack.DB {
	t.Helper()

	txDB, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("pactest: BeginTx: %v", err)
	}

	t.Cleanup(func() {
		if err := txDB.Rollback(); err != nil {
			t.Errorf("pactest: rollback: %v", err)
		}
	})

	return txDB
}
