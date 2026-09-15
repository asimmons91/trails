package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/asimmons91/trails/pack"
	"github.com/asimmons91/trails/pack/drivers/sqlite"
	"github.com/asimmons91/trails/pack/migrate"
)

type mUserV1 struct {
	pack.Model[int64] `db:"table:mig_users"`
	Name              string `db:"name,not_null"`
}

type mUserV2 struct {
	pack.Model[int64] `db:"table:mig_users"`
	Name              string `db:"name,not_null"`
	Age               int64  `db:"age"`
}

type mUserV3 struct {
	pack.Model[int64] `db:"table:mig_users"`
	Name              string `db:"name,not_null"`
	Age               int64  `db:"age,not_null"`
}

type mOrder struct {
	pack.Model[int64] `db:"table:mig_orders"`
	UserID            int64 `db:"user_id,not_null"`
	Total             int64 `db:"total"`
}

// TestMigrator_SQLite_FullRoundTrip exercises pack/migrate end to end against
// a real SQLite database (file-backed, not :memory:, so every pooled
// connection sees the same data - relevant because AlterColumn's rebuild
// path pins its own connection). It specifically proves the table-rebuild
// path used for AlterColumn (SQLite has no native ALTER COLUMN) preserves
// existing data and recreates indexes that DROP TABLE cascaded away.
func TestMigrator_SQLite_FullRoundTrip(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "migrator.db")

	db, err := pack.Connect(sqlite.New(), dsn)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	m := migrate.New(db)

	if err := m.CreateTable(ctx, &mUserV1{}, &mOrder{}); err != nil {
		t.Fatalf("CreateTable: %v", err)
	}

	if has, err := m.HasTable(ctx, &mUserV1{}); err != nil || !has {
		t.Fatalf("HasTable(mig_users) = %v, %v; want true, nil", has, err)
	}

	tables, err := m.GetTables(ctx)
	if err != nil {
		t.Fatalf("GetTables: %v", err)
	}
	wantTables := map[string]bool{"mig_users": false, "mig_orders": false}
	for _, tbl := range tables {
		if _, ok := wantTables[tbl]; ok {
			wantTables[tbl] = true
		}
	}
	for tbl, found := range wantTables {
		if !found {
			t.Fatalf("GetTables() = %v, missing %q", tables, tbl)
		}
	}

	for _, name := range []string{"Alice", "Bob"} {
		if err := pack.Create(ctx, db, &mUserV1{Name: name}); err != nil {
			t.Fatalf("Create(%s): %v", name, err)
		}
	}

	if err := m.AddColumn(ctx, &mUserV2{}, "Age"); err != nil {
		t.Fatalf("AddColumn(Age): %v", err)
	}
	if has, err := m.HasColumn(ctx, &mUserV2{}, "Age"); err != nil || !has {
		t.Fatalf("HasColumn(Age) = %v, %v; want true, nil", has, err)
	}

	// Backfill so the upcoming NOT NULL AlterColumn won't violate existing rows.
	if _, err := db.ExecContext(ctx, "Raw", "", "UPDATE mig_users SET age = 30", nil); err != nil {
		t.Fatalf("backfill UPDATE: %v", err)
	}

	if err := m.CreateIndex(ctx, &mUserV2{}, "idx_mig_users_name", []string{"Name"}); err != nil {
		t.Fatalf("CreateIndex: %v", err)
	}
	if has, err := m.HasIndex(ctx, &mUserV2{}, "idx_mig_users_name"); err != nil || !has {
		t.Fatalf("HasIndex(idx_mig_users_name) = %v, %v; want true, nil", has, err)
	}

	// The rebuild: SQLite has no ALTER COLUMN, so this drops and recreates
	// the whole table under the hood.
	if err := m.AlterColumn(ctx, &mUserV3{}, "Age"); err != nil {
		t.Fatalf("AlterColumn(Age): %v", err)
	}

	cols, err := m.ColumnTypes(ctx, &mUserV3{})
	if err != nil {
		t.Fatalf("ColumnTypes: %v", err)
	}
	var sawAge bool
	for _, c := range cols {
		if c.Name == "age" {
			sawAge = true
			if c.Nullable {
				t.Fatalf("age column is still nullable after AlterColumn(NotNull)")
			}
		}
	}
	if !sawAge {
		t.Fatalf("ColumnTypes() = %v, missing age column", cols)
	}

	if has, err := m.HasIndex(ctx, &mUserV3{}, "idx_mig_users_name"); err != nil || !has {
		t.Fatalf("HasIndex after rebuild = %v, %v; want true, nil - the index must survive the rebuild", has, err)
	}

	rows, err := pack.Raw[mUserV3](ctx, db, "SELECT id, name, age FROM mig_users ORDER BY id")
	if err != nil {
		t.Fatalf("Raw select after rebuild: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows after rebuild = %d, want 2 - data must survive the rebuild", len(rows))
	}
	for _, r := range rows {
		if r.Age != 30 {
			t.Fatalf("row %d: Age = %d, want 30 (backfilled value must survive the rebuild)", r.ID, r.Age)
		}
	}

	if err := m.DropIndex(ctx, &mUserV3{}, "idx_mig_users_name"); err != nil {
		t.Fatalf("DropIndex: %v", err)
	}
	if has, err := m.HasIndex(ctx, &mUserV3{}, "idx_mig_users_name"); err != nil || has {
		t.Fatalf("HasIndex after DropIndex = %v, %v; want false, nil", has, err)
	}
	// Idempotent: dropping again must not error.
	if err := m.DropIndex(ctx, &mUserV3{}, "idx_mig_users_name"); err != nil {
		t.Fatalf("DropIndex (idempotent no-op) returned an error: %v", err)
	}

	if err := m.DropColumn(ctx, &mUserV3{}, "Age"); err != nil {
		t.Fatalf("DropColumn(Age): %v", err)
	}
	if has, err := m.HasColumn(ctx, &mUserV3{}, "Age"); err != nil || has {
		t.Fatalf("HasColumn after DropColumn = %v, %v; want false, nil", has, err)
	}

	dbName, err := m.CurrentDatabase(ctx)
	if err != nil {
		t.Fatalf("CurrentDatabase: %v", err)
	}
	if dbName == "" {
		t.Fatalf("CurrentDatabase() = %q, want the DB file path", dbName)
	}

	if err := m.DropTable(ctx, &mUserV1{}); err != nil {
		t.Fatalf("DropTable(mig_users): %v", err)
	}
	if err := m.DropTable(ctx, &mOrder{}); err != nil {
		t.Fatalf("DropTable(mig_orders): %v", err)
	}
	if has, err := m.HasTable(ctx, &mUserV1{}); err != nil || has {
		t.Fatalf("HasTable after DropTable = %v, %v; want false, nil", has, err)
	}
	// Idempotent: dropping an already-absent table must not error.
	if err := m.DropTable(ctx, &mUserV1{}); err != nil {
		t.Fatalf("DropTable (idempotent no-op) returned an error: %v", err)
	}
}

// TestMigrator_SQLite_AlterColumn_InsideTransaction_Errors proves the
// top-level-connection guard: SQLite's PRAGMA foreign_keys cannot be
// toggled once a transaction is open, so AlterColumn's rebuild path refuses
// to run nested inside the caller's own transaction rather than silently
// skipping FK enforcement.
func TestMigrator_SQLite_AlterColumn_InsideTransaction_Errors(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "migrator_tx.db")

	db, err := pack.Connect(sqlite.New(), dsn)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	m := migrate.New(db)
	if err := m.CreateTable(ctx, &mUserV1{}); err != nil {
		t.Fatalf("CreateTable: %v", err)
	}

	err = db.Tx(ctx, func(tx *pack.DB) error {
		return migrate.New(tx).AlterColumn(ctx, &mUserV2{}, "Age")
	})
	if err != migrate.ErrAlterColumnRequiresTopLevelConnection {
		t.Fatalf("AlterColumn inside Tx error = %v, want ErrAlterColumnRequiresTopLevelConnection", err)
	}
}
