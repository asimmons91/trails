package migrate

import (
	"context"
	"database/sql/driver"
	"testing"

	"github.com/asimmons91/trails/pack/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTables_ScansAllRows(t *testing.T) {
	db, fake := newMigTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"table_name"},
		Rows: [][]driver.Value{
			testdb.Row("orders"),
			testdb.Row("users"),
			testdb.Row("widgets"),
		},
	})

	tables, err := New(db).GetTables(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"orders", "users", "widgets"}, tables)
}

func TestColumnTypes_ScansAllColumns(t *testing.T) {
	db, fake := newMigTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"column_name", "data_type", "nullable", "is_primary_key"},
		Rows: [][]driver.Value{
			testdb.Row("id", "bigint", false, true),
			testdb.Row("name", "text", false, false),
		},
	})

	cols, err := New(db).ColumnTypes(context.Background(), &migWidget{})
	require.NoError(t, err)
	require.Len(t, cols, 2)
	assert.Equal(t, ColumnType{Name: "id", SQLType: "bigint", Nullable: false, PrimaryKey: true}, cols[0])
	assert.Equal(t, ColumnType{Name: "name", SQLType: "text", Nullable: false, PrimaryKey: false}, cols[1])
}

func TestCurrentDatabase_ScansSingleValue(t *testing.T) {
	db, fake := newMigTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"current_database"},
		Rows:    [][]driver.Value{testdb.Row("pack_test")},
	})

	name, err := New(db).CurrentDatabase(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "pack_test", name)
}
