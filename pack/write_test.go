package pack

import (
	"context"
	"database/sql/driver"
	"testing"

	"github.com/asimmons91/trails/pack/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testTaggedNote struct {
	Model[int64] `db:"table:tagged_notes"`
	Body         string   `db:"body"`
	Tags         []string `db:",type:jsonb"`
}

func TestCreate_JSONBField_MarshalsBeforeBinding_R12_5(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id"},
		Rows:    [][]driver.Value{testdb.Row(int64(1))},
	})

	row := &testTaggedNote{Body: "hi", Tags: []string{"a", "b"}}
	err := Create(context.Background(), db, row)
	require.NoError(t, err)

	executed := fake.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t,
		`INSERT INTO "tagged_notes" ("body", "tags") VALUES ($1, $2) RETURNING "id"`,
		executed[0].SQL)
	require.Len(t, executed[0].Args, 2)
	assert.Equal(t, "hi", executed[0].Args[0])
	assert.Equal(t, []byte(`["a","b"]`), executed[0].Args[1])
}

func TestCreate_JSONBField_EmptySlice_MarshalsAsEmptyArray_R12_5(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id"},
		Rows:    [][]driver.Value{testdb.Row(int64(1))},
	})

	row := &testTaggedNote{Body: "hi"}
	err := Create(context.Background(), db, row)
	require.NoError(t, err)

	executed := fake.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t, []byte(`[]`), executed[0].Args[1])
}
