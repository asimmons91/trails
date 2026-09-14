package sqlbuild

import (
	"context"
	"database/sql/driver"
	"testing"

	"github.com/asimmons91/trails/pack/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests are the proof that "SQL is correct" means more than
// "the string looks right": each renders a sqlbuild statement, executes it
// through testdb.FakeDB over real database/sql (QueryContext/ExecContext),
// and asserts the driver observed exactly the rendered (sql, args) — then
// scans a canned row back out, without internal/scan existing yet.

func TestExec_Select_ThroughRealDatabaseSQL(t *testing.T) {
	sql, args, err := Select(users).
		Where(Eq(Col("email"), "a@b.com")).
		Render(pg)
	require.NoError(t, err)

	db := testdb.New()
	sqlDB := db.Open()
	defer sqlDB.Close()

	db.Enqueue(testdb.Result{
		Columns: []string{"id", "email"},
		Rows: [][]driver.Value{
			testdb.Row(int64(1), "a@b.com"),
		},
	})

	rows, err := sqlDB.QueryContext(context.Background(), sql, args...)
	require.NoError(t, err)
	defer rows.Close()

	require.True(t, rows.Next())
	var id int64
	var email string
	require.NoError(t, rows.Scan(&id, &email))
	assert.EqualValues(t, 1, id)
	assert.Equal(t, "a@b.com", email)
	require.False(t, rows.Next())

	executed := db.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t, sql, executed[0].SQL)
	assert.Equal(t, args, executed[0].Args)
}

func TestExec_InsertReturning_ThroughRealDatabaseSQL(t *testing.T) {
	sql, args, err := Insert(Table{Name: "users"}).
		Values(Set(Col("email"), "a@b.com")).
		Returning(Col("id")).
		Render(pg)
	require.NoError(t, err)

	db := testdb.New()
	sqlDB := db.Open()
	defer sqlDB.Close()

	db.Enqueue(testdb.Result{
		Columns: []string{"id"},
		Rows:    [][]driver.Value{testdb.Row(int64(42))},
	})

	rows, err := sqlDB.QueryContext(context.Background(), sql, args...)
	require.NoError(t, err)
	defer rows.Close()

	require.True(t, rows.Next())
	var id int64
	require.NoError(t, rows.Scan(&id))
	assert.EqualValues(t, 42, id)

	executed := db.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t, sql, executed[0].SQL)
	assert.Equal(t, args, executed[0].Args)
}
