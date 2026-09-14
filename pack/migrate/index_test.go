package migrate

import (
	"context"
	"database/sql/driver"
	"testing"

	"github.com/asimmons91/trails/pack/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateIndex_ByGoFieldNames(t *testing.T) {
	db, fake := newMigTestDB()
	fake.Enqueue(testdb.Result{})

	err := New(db).CreateIndex(context.Background(), &migWidget{}, "idx_widgets_name", []string{"Name"})
	require.NoError(t, err)
	assert.Equal(t, `CREATE INDEX "idx_widgets_name" ON "widgets" ("name")`, fake.Executed()[0].SQL)
}

func TestCreateIndex_Unique(t *testing.T) {
	db, fake := newMigTestDB()
	fake.Enqueue(testdb.Result{})

	err := New(db).CreateIndex(context.Background(), &migWidget{}, "idx_widgets_email", []string{"Email"}, WithUniqueIndex())
	require.NoError(t, err)
	assert.Equal(t, `CREATE UNIQUE INDEX "idx_widgets_email" ON "widgets" ("email")`, fake.Executed()[0].SQL)
}

func TestDropIndex_Idempotent_DropsWhenIndexExists(t *testing.T) {
	db, fake := newMigTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"exists"}, Rows: [][]driver.Value{testdb.Row(true)}})
	fake.Enqueue(testdb.Result{})

	err := New(db).DropIndex(context.Background(), &migWidget{}, "idx_widgets_email")
	require.NoError(t, err)
	assert.Equal(t, `DROP INDEX "idx_widgets_email"`, fake.Executed()[1].SQL)
}

func TestDropIndex_Idempotent_NoOpsWhenIndexAbsent(t *testing.T) {
	db, fake := newMigTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"exists"}, Rows: [][]driver.Value{testdb.Row(false)}})

	err := New(db).DropIndex(context.Background(), &migWidget{}, "idx_widgets_email")
	require.NoError(t, err)
	require.Len(t, fake.Executed(), 1)
}

func TestHasIndex_ScansExistsColumn(t *testing.T) {
	db, fake := newMigTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"exists"}, Rows: [][]driver.Value{testdb.Row(true)}})

	exists, err := New(db).HasIndex(context.Background(), &migWidget{}, "idx_widgets_email")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestRenameIndex_RendersExpectedSQL(t *testing.T) {
	db, fake := newMigTestDB()
	fake.Enqueue(testdb.Result{})

	err := New(db).RenameIndex(context.Background(), &migWidget{}, "old_idx", "new_idx")
	require.NoError(t, err)
	assert.Equal(t, `ALTER INDEX "old_idx" RENAME TO "new_idx"`, fake.Executed()[0].SQL)
}
