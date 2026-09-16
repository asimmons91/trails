package pactest

import (
	"context"
	"testing"

	"github.com/asimmons91/trails/pack"
)

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
