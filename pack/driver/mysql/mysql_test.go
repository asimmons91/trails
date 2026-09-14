package mysql_test

import (
	"context"
	"errors"
	"testing"

	"github.com/asimmons91/trails/driver/mysql"
	"github.com/asimmons91/trails/pack"
	"github.com/asimmons91/trails/pack/driver"
	"github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
)

func TestOpen(t *testing.T) {
	ctx := context.Background()

	mc, err := tcmysql.Run(ctx,
		"mysql:8",
		tcmysql.WithDatabase("pack"),
		tcmysql.WithUsername("pack"),
		tcmysql.WithPassword("pack"),
	)
	testcontainers.CleanupContainer(t, mc)
	if err != nil {
		t.Fatalf("start mysql container: %v", err)
	}

	connStr, err := mc.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("ConnectionString: %v", err)
	}

	db, err := mysql.New().Open(connStr)
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
	if got := mysql.New().Name(); got != "mysql" {
		t.Fatalf("Name() = %q, want %q", got, "mysql")
	}
}

func TestDialect(t *testing.T) {
	if got := mysql.New().Dialect().Name(); got != "mysql" {
		t.Fatalf("Dialect().Name() = %q, want %q", got, "mysql")
	}
}

func TestDialect_ClassifiesDuplicateKeyError(t *testing.T) {
	ctx := context.Background()

	mc, err := tcmysql.Run(ctx,
		"mysql:8",
		tcmysql.WithDatabase("pack"),
		tcmysql.WithUsername("pack"),
		tcmysql.WithPassword("pack"),
	)
	testcontainers.CleanupContainer(t, mc)
	if err != nil {
		t.Fatalf("start mysql container: %v", err)
	}

	connStr, err := mc.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("ConnectionString: %v", err)
	}

	db, err := mysql.New().Open(connStr)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx, "CREATE TABLE widgets (id BIGINT PRIMARY KEY)"); err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO widgets (id) VALUES (1)"); err != nil {
		t.Fatalf("first INSERT: %v", err)
	}

	_, err = db.ExecContext(ctx, "INSERT INTO widgets (id) VALUES (1)")
	if err == nil {
		t.Fatal("expected a duplicate-key error, got nil")
	}

	var d driver.Driver = mysql.New()
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

	mc, err := tcmysql.Run(ctx,
		"mysql:8",
		tcmysql.WithDatabase("pack"),
		tcmysql.WithUsername("pack"),
		tcmysql.WithPassword("pack"),
	)
	testcontainers.CleanupContainer(t, mc)
	if err != nil {
		t.Fatalf("start mysql container: %v", err)
	}

	connStr, err := mc.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("ConnectionString: %v", err)
	}

	rawDB, err := mysql.New().Open(connStr)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := rawDB.ExecContext(ctx, "CREATE TABLE widgets (id BIGINT PRIMARY KEY)"); err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}
	rawDB.Close()

	db, err := pack.Connect(mysql.New(), connStr)
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
