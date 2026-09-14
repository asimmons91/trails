package mysql_test

import (
	"context"
	"testing"

	"github.com/asimmons91/trails/driver/mysql"
	"github.com/asimmons91/trails/pack/dialect"
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

	d := mysql.New().Dialect()
	decoder, ok := d.(dialect.ErrorDecoder)
	if !ok {
		t.Fatalf("%T does not implement dialect.ErrorDecoder", d)
	}

	code, ok := decoder.Classify(err)
	if !ok {
		t.Fatalf("Classify(%v) = (_, false), want ok", err)
	}
	if code != dialect.CodeUnique {
		t.Fatalf("Classify(%v) code = %q, want %q", err, code, dialect.CodeUnique)
	}
}
