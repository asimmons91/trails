package migrate

import (
	"context"
	"database/sql/driver"
	"testing"

	"github.com/asimmons91/trails/pack"
	"github.com/asimmons91/trails/pack/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type migSQLiteWidget struct {
	ID   int64  `db:"id,pk,auto_increment"`
	Name string `db:"name,not_null"`
}

func (migSQLiteWidget) TableName() string { return "widgets" }

func TestAlterColumn_SQLite_RunsFullRebuildSequence(t *testing.T) {
	db, fake := newMigTestSQLiteDB()

	fake.Enqueue(testdb.Result{})                                                // PRAGMA foreign_keys = OFF
	fake.Enqueue(testdb.Result{Columns: []string{"sql"}, Rows: [][]driver.Value{ // captured index SQL
		testdb.Row(`CREATE INDEX "idx_widgets_name" ON "widgets" ("name")`),
	}})
	fake.Enqueue(testdb.Result{})                           // CREATE TABLE widgets__pack_rebuild
	fake.Enqueue(testdb.Result{})                           // INSERT INTO ... SELECT
	fake.Enqueue(testdb.Result{})                           // DROP TABLE widgets
	fake.Enqueue(testdb.Result{})                           // ALTER TABLE ... RENAME TO widgets
	fake.Enqueue(testdb.Result{})                           // replayed CREATE INDEX
	fake.Enqueue(testdb.Result{Columns: []string{"table"}}) // PRAGMA foreign_key_check (no violations)
	fake.Enqueue(testdb.Result{})                           // PRAGMA foreign_keys = ON

	err := New(db).AlterColumn(context.Background(), &migSQLiteWidget{}, "Name")
	require.NoError(t, err)

	executed := fake.Executed()
	require.Len(t, executed, 9)
	assert.Equal(t, "PRAGMA foreign_keys = OFF", executed[0].SQL)
	assert.Contains(t, executed[1].SQL, "sqlite_master")
	assert.Equal(t, `CREATE TABLE "widgets__pack_rebuild" ("id" INTEGER PRIMARY KEY AUTOINCREMENT, "name" TEXT NOT NULL)`, executed[2].SQL)
	assert.Equal(t, `INSERT INTO "widgets__pack_rebuild" ("id", "name") SELECT "id", "name" FROM "widgets"`, executed[3].SQL)
	assert.Equal(t, `DROP TABLE "widgets"`, executed[4].SQL)
	assert.Equal(t, `ALTER TABLE "widgets__pack_rebuild" RENAME TO "widgets"`, executed[5].SQL)
	assert.Equal(t, `CREATE INDEX "idx_widgets_name" ON "widgets" ("name")`, executed[6].SQL)
	assert.Equal(t, "PRAGMA foreign_key_check", executed[7].SQL)
	assert.Equal(t, "PRAGMA foreign_keys = ON", executed[8].SQL)

	txEvents := fake.TxEvents()
	require.Len(t, txEvents, 2)
	assert.Equal(t, "BEGIN", txEvents[0].Kind)
	assert.Equal(t, "COMMIT", txEvents[1].Kind)
}

func TestAlterColumn_SQLite_ForeignKeyCheckViolation_ReturnsError(t *testing.T) {
	db, fake := newMigTestSQLiteDB()

	fake.Enqueue(testdb.Result{})                         // PRAGMA foreign_keys = OFF
	fake.Enqueue(testdb.Result{Columns: []string{"sql"}}) // no indexes to capture
	fake.Enqueue(testdb.Result{})                         // CREATE TABLE
	fake.Enqueue(testdb.Result{})                         // INSERT
	fake.Enqueue(testdb.Result{})                         // DROP TABLE
	fake.Enqueue(testdb.Result{})                         // RENAME
	fake.Enqueue(testdb.Result{                           // PRAGMA foreign_key_check finds a violation
		Columns: []string{"table", "rowid", "parent", "fkid"},
		Rows:    [][]driver.Value{testdb.Row("orders", int64(1), "widgets", int64(0))},
	})

	err := New(db).AlterColumn(context.Background(), &migSQLiteWidget{}, "Name")
	require.Error(t, err)
	var fkErr *ErrForeignKeyCheckFailed
	require.ErrorAs(t, err, &fkErr)
	assert.Len(t, fkErr.Rows, 1)

	// PRAGMA foreign_keys = ON must NOT have run: the failure short-circuits.
	last := fake.Executed()[len(fake.Executed())-1]
	assert.NotEqual(t, "PRAGMA foreign_keys = ON", last.SQL)
}

func TestAlterColumn_SQLite_InsideExistingTransaction_ReturnsError(t *testing.T) {
	db, _ := newMigTestSQLiteDB()

	err := db.Tx(context.Background(), func(tx *pack.DB) error {
		return New(tx).AlterColumn(context.Background(), &migSQLiteWidget{}, "Name")
	})
	require.ErrorIs(t, err, ErrAlterColumnRequiresTopLevelConnection)
}
