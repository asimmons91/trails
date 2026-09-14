package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/asimmons91/trails/pack"
	"github.com/asimmons91/trails/pack/driver"
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

func TestDialect_ClassifiesDuplicateKeyError(t *testing.T) {
	db, err := sqlite.New().Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec("CREATE TABLE widgets (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}
	if _, err := db.Exec("INSERT INTO widgets (id) VALUES (1)"); err != nil {
		t.Fatalf("first INSERT: %v", err)
	}

	_, err = db.Exec("INSERT INTO widgets (id) VALUES (1)")
	if err == nil {
		t.Fatal("expected a duplicate-key error, got nil")
	}

	var d driver.Driver = sqlite.New()
	decoder, ok := d.(driver.ErrorDecoder)
	if !ok {
		t.Fatalf("%T does not implement driver.ErrorDecoder", d)
	}

	code, ok := decoder.Classify(err)
	if !ok {
		t.Fatalf("Classify(%v) = (_, false), want ok", err)
	}
	if code != driver.CodeUnique {
		t.Fatalf("Classify(%v) code = %q, want %q", err, code, driver.CodeUnique)
	}
}

type widget struct {
	ID int64 `db:"id,pk"`
}

func (widget) TableName() string { return "widgets" }

func TestConnect_ClassifiesDuplicateKeyError(t *testing.T) {
	ctx := context.Background()

	dsn := filepath.Join(t.TempDir(), "test.db")

	rawDB, err := sqlite.New().Open(dsn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := rawDB.Exec("CREATE TABLE widgets (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}
	rawDB.Close()

	db, err := pack.Connect(sqlite.New(), dsn)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	if err := pack.Create(ctx, db, &widget{ID: 1}); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	err = pack.Create(ctx, db, &widget{ID: 1})
	if err == nil {
		t.Fatal("expected a duplicate-key error, got nil")
	}

	if _, ok := errors.AsType[*pack.ErrUniqueViolation](err); !ok {
		t.Fatalf("Create error = %v, want *pack.ErrUniqueViolation", err)
	}
}
