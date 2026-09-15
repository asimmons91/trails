package migrate

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/asimmons91/trails/pack"
)

// schemaMigrationRow tracks which registered Migrations have already been
// applied. It's a plain pack-mapped model (not pack.Model[string]: the
// embedded Model's ID field is always tagged auto_increment, which is only
// valid for integer primary keys) so it can be created and queried with the
// same Migrator/pack APIs a consumer would use.
//
// AppliedAt is a Unix nanosecond timestamp rather than a time.Time: sqlite
// dialect maps time.Time to a plain TEXT column with no declared date type,
// which the sqlite driver then can't scan back into time.Time. An int64
// sorts and round-trips identically across all three dialects.
type schemaMigrationRow struct {
	ID        string `db:"id,pk"`
	AppliedAt int64  `db:"applied_at,not_null"`
}

func (schemaMigrationRow) TableName() string { return "schema_migrations" }

var migrationRowCol = struct {
	ID        pack.Col[schemaMigrationRow, string]
	AppliedAt pack.Col[schemaMigrationRow, int64]
}{
	ID:        pack.Field(func(r *schemaMigrationRow) *string { return &r.ID }),
	AppliedAt: pack.Field(func(r *schemaMigrationRow) *int64 { return &r.AppliedAt }),
}

// Up applies every registered migration that has not yet been recorded in
// schema_migrations, in ID order. A migration that fails leaves every
// migration applied before it in place: migrations run against a top-level
// *Migrator rather than inside a transaction, since SQLite's AlterColumn
// requires a top-level connection and MySQL DDL isn't transactional anyway.
func Up(ctx context.Context, db *pack.DB) error {
	m := New(db)
	if err := ensureMigrationsTable(ctx, m); err != nil {
		return err
	}

	applied, err := appliedIDs(ctx, db)
	if err != nil {
		return err
	}

	for _, mig := range Registered() {
		if applied[mig.ID] {
			continue
		}

		if err := mig.Migrate(ctx, m); err != nil {
			return fmt.Errorf("pack/migrate: migrating %s: %w", mig.ID, err)
		}

		row := schemaMigrationRow{ID: mig.ID, AppliedAt: time.Now().UnixNano()}
		if err := pack.Create(ctx, db, &row); err != nil {
			return fmt.Errorf("pack/migrate: recording %s: %w", mig.ID, err)
		}
	}

	return nil
}

// Down rolls back the single most recently applied migration. It is a no-op
// if no migration has been applied.
func Down(ctx context.Context, db *pack.DB) error {
	m := New(db)
	if err := ensureMigrationsTable(ctx, m); err != nil {
		return err
	}

	row, err := pack.Of[schemaMigrationRow](db).Order(migrationRowCol.AppliedAt.Desc()).First(ctx)
	if err != nil {
		if errors.Is(err, pack.ErrNoRows) {
			return nil
		}
		return err
	}

	mig, ok := findRegistered(row.ID)
	if !ok {
		return fmt.Errorf(
			"pack/migrate: schema_migrations records %q as applied, but no migration with that ID is registered",
			row.ID,
		)
	}

	if err := mig.Rollback(ctx, m); err != nil {
		return fmt.Errorf("pack/migrate: rolling back %s: %w", mig.ID, err)
	}

	if _, err := pack.Of[schemaMigrationRow](db).Where(migrationRowCol.ID.Eq(row.ID)).Delete(ctx); err != nil {
		return fmt.Errorf("pack/migrate: removing record for %s: %w", mig.ID, err)
	}

	return nil
}

func ensureMigrationsTable(ctx context.Context, m *Migrator) error {
	has, err := m.HasTable(ctx, &schemaMigrationRow{})
	if err != nil {
		return err
	}
	if has {
		return nil
	}

	return m.CreateTable(ctx, &schemaMigrationRow{})
}

func appliedIDs(ctx context.Context, db *pack.DB) (map[string]bool, error) {
	rows, err := pack.Of[schemaMigrationRow](db).Find(ctx)
	if err != nil {
		return nil, err
	}

	ids := make(map[string]bool, len(rows))
	for _, r := range rows {
		ids[r.ID] = true
	}

	return ids, nil
}

func findRegistered(id string) (Migration, bool) {
	for _, mig := range Registered() {
		if mig.ID == id {
			return mig, true
		}
	}

	return Migration{}, false
}
