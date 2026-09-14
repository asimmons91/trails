package pack

import (
	"context"
	"database/sql/driver"
	"testing"

	"github.com/asimmons91/trails/pack/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRaw_ExecutesVerbatimSQL_AndScansViaNormalMapper(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "email", "age"},
		Rows: [][]driver.Value{
			testdb.Row(int64(1), "a@b.com", int64(30)),
		},
	})

	got, err := Raw[testUser](context.Background(), db,
		`SELECT id, email, age FROM users WHERE age > $1`, 18)
	require.NoError(t, err)
	assert.Equal(t, []testUser{{ID: 1, Email: "a@b.com", Age: 30}}, got)

	executed := fake.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t, `SELECT id, email, age FROM users WHERE age > $1`, executed[0].SQL)
	assert.Equal(t, []any{18}, executed[0].Args)
}

func TestRaw_UnmappedExtraColumn_IsDiscardedNotError(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "email", "age", "rank"},
		Rows: [][]driver.Value{
			testdb.Row(int64(1), "a@b.com", int64(30), int64(7)),
		},
	})

	got, err := Raw[testUser](context.Background(), db,
		`SELECT id, email, age, rank() OVER (ORDER BY age) AS rank FROM users`)
	require.NoError(t, err)
	assert.Equal(t, []testUser{{ID: 1, Email: "a@b.com", Age: 30}}, got)
}
