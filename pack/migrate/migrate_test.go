package migrate

import (
	"context"
	"database/sql/driver"
	"testing"

	"github.com/asimmons91/trails/pack"
	"github.com/asimmons91/trails/pack/dialect/pgdialect"
	"github.com/asimmons91/trails/pack/dialect/sqlitedialect"
	"github.com/asimmons91/trails/pack/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type migWidget struct {
	pack.Model[int64] `db:"table:widgets"`
	Name              string `db:"name,not_null"`
	Email             string `db:"email,unique"`
}

type migUser struct {
	pack.Model[int64] `db:"table:users"`
	Name              string `db:"name"`
}

type migOrder struct {
	pack.Model[int64] `db:"table:orders"`
	UserID            int64    `db:"user_id"`
	User              *migUser `db:"rel:belongs_to,fk:user_id,ref:id"`
}

type notATable struct {
	V chan int
}

func newMigTestDB() (*pack.DB, *testdb.FakeDB) {
	fake := testdb.New()
	db := pack.Open(fake.Open(), pgdialect.New())
	return db, fake
}

func newMigTestSQLiteDB() (*pack.DB, *testdb.FakeDB) {
	fake := testdb.New()
	db := pack.Open(fake.Open(), sqlitedialect.New())
	return db, fake
}

func TestNew_PanicsWhenDialectHasNoDDLSupport(t *testing.T) {
	fake := testdb.New()
	db := pack.Open(fake.Open(), noDDLDialect{})
	assert.Panics(t, func() { New(db) })
}

// noDDLDialect implements dialect.Dialect but deliberately not dialect.DDL,
// to exercise New's guard against dialects with no Migrator support.
type noDDLDialect struct{}

func (noDDLDialect) Name() string                  { return "no-ddl-fake" }
func (noDDLDialect) QuoteIdent(name string) string { return `"` + name + `"` }
func (noDDLDialect) Placeholder(n int) string      { return "$1" }
func (noDDLDialect) SupportsILike() bool           { return false }
func (noDDLDialect) SupportsRowLocking() bool      { return false }
func (noDDLDialect) SupportsReturning() bool       { return false }
func (noDDLDialect) SupportsOnConflict() bool      { return false }
func (noDDLDialect) SupportsSavepoints() bool      { return false }

func TestCreateTable_SingleModel_RendersExpectedSQL(t *testing.T) {
	db, fake := newMigTestDB()
	fake.Enqueue(testdb.Result{RowsAffected: 0})

	err := New(db).CreateTable(context.Background(), &migWidget{})
	require.NoError(t, err)

	require.Len(t, fake.Executed(), 1)
	assert.Equal(t,
		`CREATE TABLE "widgets" ("id" BIGSERIAL, "name" TEXT NOT NULL, "email" TEXT UNIQUE, PRIMARY KEY ("id"))`,
		fake.Executed()[0].SQL,
	)
}

func TestCreateTable_MultipleModels_IssuesOneStatementPerModel(t *testing.T) {
	db, fake := newMigTestDB()
	fake.Enqueue(testdb.Result{})
	fake.Enqueue(testdb.Result{})

	err := New(db).CreateTable(context.Background(), &migWidget{}, &migUser{})
	require.NoError(t, err)
	require.Len(t, fake.Executed(), 2)
}

func TestCreateTable_UnmappableStruct_Panics(t *testing.T) {
	db, _ := newMigTestDB()
	assert.Panics(t, func() {
		_ = New(db).CreateTable(context.Background(), &notATable{})
	})
}

func TestDropTable_Idempotent_NoOpsWhenTableAbsent(t *testing.T) {
	db, fake := newMigTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"exists"}, Rows: [][]driver.Value{testdb.Row(false)}})

	err := New(db).DropTable(context.Background(), &migWidget{})
	require.NoError(t, err)

	// Only the HasTable check ran; no DROP TABLE was issued.
	require.Len(t, fake.Executed(), 1)
	assert.Contains(t, fake.Executed()[0].SQL, "information_schema.tables")
}

func TestDropTable_Idempotent_DropsWhenTableExists(t *testing.T) {
	db, fake := newMigTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"exists"}, Rows: [][]driver.Value{testdb.Row(true)}})
	fake.Enqueue(testdb.Result{})

	err := New(db).DropTable(context.Background(), &migWidget{})
	require.NoError(t, err)

	require.Len(t, fake.Executed(), 2)
	assert.Equal(t, `DROP TABLE "widgets"`, fake.Executed()[1].SQL)
}

func TestDropTable_WithoutIfExists_SkipsCheckAndAlwaysDrops(t *testing.T) {
	db, fake := newMigTestDB()
	fake.Enqueue(testdb.Result{})

	err := New(db).DropTable(context.Background(), &migWidget{}, WithoutIfExists())
	require.NoError(t, err)

	require.Len(t, fake.Executed(), 1)
	assert.Equal(t, `DROP TABLE "widgets"`, fake.Executed()[0].SQL)
}

func TestHasTable_ScansExistsColumn(t *testing.T) {
	db, fake := newMigTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"exists"}, Rows: [][]driver.Value{testdb.Row(true)}})

	exists, err := New(db).HasTable(context.Background(), &migWidget{})
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestRenameTable_RendersExpectedSQL(t *testing.T) {
	db, fake := newMigTestDB()
	fake.Enqueue(testdb.Result{})

	err := New(db).RenameTable(context.Background(), &migWidget{}, "gadgets")
	require.NoError(t, err)
	assert.Equal(t, `ALTER TABLE "widgets" RENAME TO "gadgets"`, fake.Executed()[0].SQL)
}
