package sqlite_test

import (
	"testing"

	"github.com/asimmons91/trails/pack/drivers/sqlite"
)

func TestOpen(t *testing.T) {
	db, err := sqlite.New().Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	var got int
	if err := db.QueryRow("SELECT 1").Scan(&got); err != nil {
		t.Fatalf("QueryRow: %v", err)
	}
	if got != 1 {
		t.Fatalf("got %d, want 1", got)
	}
}

func TestName(t *testing.T) {
	if got := sqlite.New().Name(); got != "sqlite" {
		t.Fatalf("Name() = %q, want %q", got, "sqlite")
	}
}

func TestDialect(t *testing.T) {
	if got := sqlite.New().Dialect().Name(); got != "sqlite" {
		t.Fatalf("Dialect().Name() = %q, want %q", got, "sqlite")
	}
}
