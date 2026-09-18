package migrate

import (
	"context"
	"database/sql/driver"
	"fmt"

	"github.com/asimmons91/trails/pack/internal/testdb"
)

// ExampleMigrator_AlterColumn shows what happens on SQLite, which has no
// native ALTER COLUMN: instead of one statement, AlterColumn transparently
// rebuilds the whole table — create a shadow table with the new column
// shapes, copy every row across, drop the original, rename the shadow into
// place, then replay its indexes — all inside one transaction.
func ExampleMigrator_AlterColumn() {
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

	if err := New(db).AlterColumn(context.Background(), &migSQLiteWidget{}, "Name"); err != nil {
		fmt.Println("error:", err)
		return
	}

	for _, e := range fake.Executed() {
		fmt.Println(e.SQL)
	}
	// Output:
	// PRAGMA foreign_keys = OFF
	// SELECT sql FROM sqlite_master WHERE type = 'index' AND tbl_name = ?1 AND sql IS NOT NULL
	// CREATE TABLE "widgets__pack_rebuild" ("id" INTEGER PRIMARY KEY AUTOINCREMENT, "name" TEXT NOT NULL)
	// INSERT INTO "widgets__pack_rebuild" ("id", "name") SELECT "id", "name" FROM "widgets"
	// DROP TABLE "widgets"
	// ALTER TABLE "widgets__pack_rebuild" RENAME TO "widgets"
	// CREATE INDEX "idx_widgets_name" ON "widgets" ("name")
	// PRAGMA foreign_key_check
	// PRAGMA foreign_keys = ON
}
