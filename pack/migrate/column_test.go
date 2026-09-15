package migrate

import (
	"context"
	"database/sql/driver"
	"testing"

	"github.com/asimmons91/trails/pack/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddColumn_RendersExpectedSQL(t *testing.T) {
	db, fake := newMigTestDB()
	fake.Enqueue(testdb.Result{})

	err := New(db).AddColumn(context.Background(), &migWidget{}, "Email")
	require.NoError(t, err)
	assert.Equal(t, `ALTER TABLE "widgets" ADD COLUMN "email" TEXT UNIQUE`, fake.Executed()[0].SQL)
}

func TestAddColumn_UnknownField_Panics(t *testing.T) {
	db, _ := newMigTestDB()
	assert.Panics(t, func() {
		_ = New(db).AddColumn(context.Background(), &migWidget{}, "DoesNotExist")
	})
}

func TestDropColumn_ByGoFieldName(t *testing.T) {
	db, fake := newMigTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"exists"}, Rows: [][]driver.Value{testdb.Row(true)}})
	fake.Enqueue(testdb.Result{})

	err := New(db).DropColumn(context.Background(), &migWidget{}, "Email")
	require.NoError(t, err)
	require.Len(t, fake.Executed(), 2)
	assert.Equal(t, `ALTER TABLE "widgets" DROP COLUMN "email"`, fake.Executed()[1].SQL)
}

func TestDropColumn_LiteralColumnName_WhenNoStructFieldMatches(t *testing.T) {
	// legacy_flag was removed from the Go struct already, but still exists
	// in the live table - the normal real-world case for dropping a column.
	db, fake := newMigTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"exists"}, Rows: [][]driver.Value{testdb.Row(true)}})
	fake.Enqueue(testdb.Result{})

	err := New(db).DropColumn(context.Background(), &migWidget{}, "legacy_flag")
	require.NoError(t, err)
	assert.Equal(t, `ALTER TABLE "widgets" DROP COLUMN "legacy_flag"`, fake.Executed()[1].SQL)
}

func TestDropColumn_Idempotent_NoOpsWhenColumnAbsent(t *testing.T) {
	db, fake := newMigTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"exists"}, Rows: [][]driver.Value{testdb.Row(false)}})

	err := New(db).DropColumn(context.Background(), &migWidget{}, "Email")
	require.NoError(t, err)
	require.Len(t, fake.Executed(), 1, "only the HasColumn check should have run")
}

func TestHasColumn_ScansExistsColumn(t *testing.T) {
	db, fake := newMigTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"exists"}, Rows: [][]driver.Value{testdb.Row(true)}})

	exists, err := New(db).HasColumn(context.Background(), &migWidget{}, "Email")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestRenameColumn_RendersExpectedSQL(t *testing.T) {
	db, fake := newMigTestDB()
	fake.Enqueue(testdb.Result{})

	err := New(db).RenameColumn(context.Background(), &migWidget{}, "Email", "email_address")
	require.NoError(t, err)
	assert.Equal(t, `ALTER TABLE "widgets" RENAME COLUMN "email" TO "email_address"`, fake.Executed()[0].SQL)
}

func TestAlterColumn_Postgres_RendersExpectedSQL(t *testing.T) {
	db, fake := newMigTestDB()
	fake.Enqueue(testdb.Result{})

	err := New(db).AlterColumn(context.Background(), &migWidget{}, "Name")
	require.NoError(t, err)
	assert.Equal(t,
		`ALTER TABLE "widgets" ALTER COLUMN "name" TYPE TEXT, ALTER COLUMN "name" SET NOT NULL, ALTER COLUMN "name" DROP DEFAULT`,
		fake.Executed()[0].SQL,
	)
}

func TestAlterColumn_UnknownField_Panics(t *testing.T) {
	db, _ := newMigTestDB()
	assert.Panics(t, func() {
		_ = New(db).AlterColumn(context.Background(), &migWidget{}, "DoesNotExist")
	})
}
