package migrate

import (
	"context"
	"database/sql/driver"
	"testing"
	"time"

	"github.com/asimmons91/trails/pack/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withRegistered(t *testing.T, migs ...Migration) {
	t.Helper()
	saved := registered
	registered = nil
	for _, m := range migs {
		Register(m)
	}
	t.Cleanup(func() { registered = saved })
}

func hasTableResult(exists bool) testdb.Result {
	return testdb.Result{Columns: []string{"exists"}, Rows: [][]driver.Value{testdb.Row(exists)}}
}

func appliedRowsResult(rows ...[]driver.Value) testdb.Result {
	return testdb.Result{Columns: []string{"id", "applied_at"}, Rows: rows}
}

func TestUp_CreatesTrackingTableAndAppliesPendingMigrationsInIDOrder(t *testing.T) {
	db, fake := newMigTestDB()

	var order []string
	withRegistered(t,
		Migration{
			ID: "20260102000000_b",
			Migrate: func(ctx context.Context, m *Migrator) error {
				order = append(order, "20260102000000_b")
				return nil
			},
		},
		Migration{
			ID: "20260101000000_a",
			Migrate: func(ctx context.Context, m *Migrator) error {
				order = append(order, "20260101000000_a")
				return nil
			},
		},
	)

	fake.Enqueue(hasTableResult(false))          // ensureMigrationsTable: HasTable
	fake.Enqueue(testdb.Result{})                // ensureMigrationsTable: CreateTable
	fake.Enqueue(appliedRowsResult())            // appliedIDs: Find (none applied yet)
	fake.Enqueue(testdb.Result{RowsAffected: 1}) // Create tracking row for "a"
	fake.Enqueue(testdb.Result{RowsAffected: 1}) // Create tracking row for "b"

	err := Up(context.Background(), db)
	require.NoError(t, err)

	assert.Equal(t, []string{"20260101000000_a", "20260102000000_b"}, order)
	require.Len(t, fake.Executed(), 5)
}

func TestUp_SkipsAlreadyAppliedMigrations(t *testing.T) {
	db, fake := newMigTestDB()

	var ran []string
	withRegistered(t,
		Migration{
			ID: "20260101000000_a",
			Migrate: func(ctx context.Context, m *Migrator) error {
				ran = append(ran, "20260101000000_a")
				return nil
			},
		},
		Migration{
			ID: "20260102000000_b",
			Migrate: func(ctx context.Context, m *Migrator) error {
				ran = append(ran, "20260102000000_b")
				return nil
			},
		},
	)

	fake.Enqueue(hasTableResult(true)) // ensureMigrationsTable: table already exists
	fake.Enqueue(appliedRowsResult(testdb.Row("20260101000000_a", time.Now().UnixNano())))
	fake.Enqueue(testdb.Result{RowsAffected: 1}) // Create tracking row for "b" only

	err := Up(context.Background(), db)
	require.NoError(t, err)

	assert.Equal(t, []string{"20260102000000_b"}, ran)
	require.Len(t, fake.Executed(), 3)
}

func TestUp_MigrationError_StopsWithoutRecordingIt(t *testing.T) {
	db, fake := newMigTestDB()

	boom := assert.AnError
	withRegistered(t, Migration{
		ID: "20260101000000_a",
		Migrate: func(ctx context.Context, m *Migrator) error {
			return boom
		},
	})

	fake.Enqueue(hasTableResult(true))
	fake.Enqueue(appliedRowsResult())

	err := Up(context.Background(), db)
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
	require.Len(t, fake.Executed(), 2, "no INSERT should run for a migration that failed")
}

func TestDown_RollsBackOnlyMostRecent(t *testing.T) {
	db, fake := newMigTestDB()

	var rolledBack []string
	withRegistered(t, Migration{
		ID: "20260101000000_a",
		Rollback: func(ctx context.Context, m *Migrator) error {
			rolledBack = append(rolledBack, "20260101000000_a")
			return nil
		},
	})

	fake.Enqueue(hasTableResult(true))
	fake.Enqueue(appliedRowsResult(testdb.Row("20260101000000_a", time.Now().UnixNano()))) // First
	fake.Enqueue(testdb.Result{RowsAffected: 1})                                           // Delete

	err := Down(context.Background(), db)
	require.NoError(t, err)
	assert.Equal(t, []string{"20260101000000_a"}, rolledBack)
}

func TestDown_NoAppliedMigrations_NoOp(t *testing.T) {
	db, fake := newMigTestDB()
	withRegistered(t)

	fake.Enqueue(hasTableResult(true))
	fake.Enqueue(appliedRowsResult()) // First: no rows

	err := Down(context.Background(), db)
	require.NoError(t, err)
	require.Len(t, fake.Executed(), 2, "no ROLLBACK or DELETE should run")
}

func TestDown_UnregisteredAppliedID_ReturnsError(t *testing.T) {
	db, fake := newMigTestDB()
	withRegistered(t) // nothing registered, but schema_migrations records one

	fake.Enqueue(hasTableResult(true))
	fake.Enqueue(appliedRowsResult(testdb.Row("20260101000000_orphan", time.Now().UnixNano())))

	err := Down(context.Background(), db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "20260101000000_orphan")
	require.Len(t, fake.Executed(), 2, "no DELETE should run once the migration can't be found")
}
