package mysql_test

import (
	"context"
	"errors"
	"testing"

	"github.com/asimmons91/trails/pack"
	"github.com/asimmons91/trails/pack/driver/mysql"
	"github.com/asimmons91/trails/pack/migrate"
	"github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
)

type myUser struct {
	pack.Model[int64] `db:"table:mig_users"`
	Name              string `db:"name,not_null"`
}

// myUserWithNullableAge is the shape used for AddColumn: the column must
// start nullable since the table already has rows and MySQL's default
// strict mode rejects a NOT NULL ADD COLUMN with no DEFAULT on a non-empty
// table.
type myUserWithNullableAge struct {
	pack.Model[int64] `db:"table:mig_users"`
	Name              string `db:"name,not_null"`
	Age               int64  `db:"age"`
}

// myUserWithAge is the target shape for AlterColumn, once every row has
// been backfilled with a real Age value.
type myUserWithAge struct {
	pack.Model[int64] `db:"table:mig_users"`
	Name              string `db:"name,not_null"`
	Age               int64  `db:"age,not_null"`
}

type myOrder struct {
	pack.Model[int64] `db:"table:mig_orders"`
	UserID            int64   `db:"user_id,not_null"`
	User              *myUser `db:"rel:belongs_to,fk:user_id,ref:id"`
}

// TestMigrator_MySQL_FullRoundTrip exercises pack/migrate end to end against
// a real MySQL container: table/column/index/constraint lifecycle,
// introspection, and that the FK constraint it creates is actually enforced
// by the database, not just recorded as SQL text.
func TestMigrator_MySQL_FullRoundTrip(t *testing.T) {
	t.Parallel()
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

	db, err := pack.Connect(mysql.New(), connStr)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	m := migrate.New(db)

	if err := m.CreateTable(ctx, &myUser{}, &myOrder{}); err != nil {
		t.Fatalf("CreateTable: %v", err)
	}

	if has, err := m.HasTable(ctx, &myUser{}); err != nil || !has {
		t.Fatalf("HasTable(mig_users) = %v, %v; want true, nil", has, err)
	}

	for _, name := range []string{"Alice", "Bob"} {
		if err := pack.Create(ctx, db, &myUser{Name: name}); err != nil {
			t.Fatalf("Create(%s): %v", name, err)
		}
	}

	if err := m.AddColumn(ctx, &myUserWithNullableAge{}, "Age"); err != nil {
		t.Fatalf("AddColumn(Age): %v", err)
	}
	if _, err := db.ExecContext(ctx, "Raw", "", "UPDATE mig_users SET age = 30", nil); err != nil {
		t.Fatalf("backfill UPDATE: %v", err)
	}

	if err := m.CreateIndex(ctx, &myUserWithNullableAge{}, "idx_mig_users_name", []string{"Name"}); err != nil {
		t.Fatalf("CreateIndex: %v", err)
	}
	if has, err := m.HasIndex(ctx, &myUserWithNullableAge{}, "idx_mig_users_name"); err != nil || !has {
		t.Fatalf("HasIndex = %v, %v; want true, nil", has, err)
	}

	// MySQL's MODIFY COLUMN handles this as a single statement - no rebuild.
	if err := m.AlterColumn(ctx, &myUserWithAge{}, "Age"); err != nil {
		t.Fatalf("AlterColumn(Age): %v", err)
	}

	cols, err := m.ColumnTypes(ctx, &myUserWithAge{})
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

	rows, err := pack.Raw[myUserWithAge](ctx, db, "SELECT id, name, age FROM mig_users ORDER BY id")
	if err != nil {
		t.Fatalf("Raw select: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	for _, r := range rows {
		if r.Age != 30 {
			t.Fatalf("row %d: Age = %d, want 30", r.ID, r.Age)
		}
	}

	if err := m.CreateConstraint(ctx, &myOrder{}, "User"); err != nil {
		t.Fatalf("CreateConstraint: %v", err)
	}
	if has, err := m.HasConstraint(ctx, &myOrder{}, "fk_mig_orders_user_id"); err != nil || !has {
		t.Fatalf("HasConstraint = %v, %v; want true, nil", has, err)
	}

	err = pack.Create(ctx, db, &myOrder{UserID: 999999})
	if err == nil {
		t.Fatal("expected a foreign key violation inserting an order for a nonexistent user")
	}
	var fkErr *pack.ErrForeignKeyViolation
	if !errors.As(err, &fkErr) {
		t.Fatalf("Create error = %v (%T), want *pack.ErrForeignKeyViolation", err, err)
	}

	if err := m.DropConstraint(ctx, &myOrder{}, "fk_mig_orders_user_id"); err != nil {
		t.Fatalf("DropConstraint: %v", err)
	}
	if has, err := m.HasConstraint(ctx, &myOrder{}, "fk_mig_orders_user_id"); err != nil || has {
		t.Fatalf("HasConstraint after DropConstraint = %v, %v; want false, nil", has, err)
	}
	// Idempotent: dropping again must not error.
	if err := m.DropConstraint(ctx, &myOrder{}, "fk_mig_orders_user_id"); err != nil {
		t.Fatalf("DropConstraint (idempotent no-op) returned an error: %v", err)
	}

	if err := m.DropIndex(ctx, &myUserWithAge{}, "idx_mig_users_name"); err != nil {
		t.Fatalf("DropIndex: %v", err)
	}
	if err := m.DropColumn(ctx, &myUserWithAge{}, "Age"); err != nil {
		t.Fatalf("DropColumn(Age): %v", err)
	}
	if has, err := m.HasColumn(ctx, &myUserWithAge{}, "Age"); err != nil || has {
		t.Fatalf("HasColumn after DropColumn = %v, %v; want false, nil", has, err)
	}

	dbName, err := m.CurrentDatabase(ctx)
	if err != nil {
		t.Fatalf("CurrentDatabase: %v", err)
	}
	if dbName != "pack" {
		t.Fatalf("CurrentDatabase() = %q, want %q", dbName, "pack")
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

	if err := m.DropTable(ctx, &myOrder{}); err != nil {
		t.Fatalf("DropTable(mig_orders): %v", err)
	}
	if err := m.DropTable(ctx, &myUser{}); err != nil {
		t.Fatalf("DropTable(mig_users): %v", err)
	}
	if has, err := m.HasTable(ctx, &myUser{}); err != nil || has {
		t.Fatalf("HasTable after DropTable = %v, %v; want false, nil", has, err)
	}
	// Idempotent: dropping an already-absent table must not error.
	if err := m.DropTable(ctx, &myUser{}); err != nil {
		t.Fatalf("DropTable (idempotent no-op) returned an error: %v", err)
	}
}
