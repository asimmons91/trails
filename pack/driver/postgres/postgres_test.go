package postgres_test

import (
	"context"
	"testing"

	"github.com/asimmons91/trails/driver/postgres"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestOpen(t *testing.T) {
	ctx := context.Background()

	pgc, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("pack"),
		tcpostgres.WithUsername("pack"),
		tcpostgres.WithPassword("pack"),
		tcpostgres.BasicWaitStrategies(),
	)
	testcontainers.CleanupContainer(t, pgc)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}

	connStr, err := pgc.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("ConnectionString: %v", err)
	}

	db, err := postgres.New().Open(connStr)
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
	if got := postgres.New().Name(); got != "pgx" {
		t.Fatalf("Name() = %q, want %q", got, "pgx")
	}
}

func TestDialect(t *testing.T) {
	if got := postgres.New().Dialect().Name(); got != "postgres" {
		t.Fatalf("Dialect().Name() = %q, want %q", got, "postgres")
	}
}
