package pack

import (
	"context"
	"database/sql/driver"
	"testing"

	"github.com/asimmons91/trails/pack/dialect/pgdialect"
	"github.com/asimmons91/trails/pack/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestDB() (*DB, *testdb.FakeDB) {
	fake := testdb.New()
	db := Open(fake.Open(), pgdialect.New())
	return db, fake
}

func TestField_NilReturningSelector_Panics(t *testing.T) {
	assert.Panics(t, func() {
		Field(func(u *testUser) *int64 { return nil })
	})
}

func TestField_SelectorReturningPointerOutsideStruct_Panics(t *testing.T) {
	var external int64
	assert.Panics(t, func() {
		Field(func(u *testUser) *int64 {
			_ = u
			return &external
		})
	})
}

func TestField_TopLevelField_ResolvesToCorrectColumn(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"id", "email", "age"}})

	_, err := Of[testUser](db).Where(userCol.Age.Gt(18)).Find(context.Background())
	require.NoError(t, err)

	executed := fake.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t,
		`SELECT "users"."id", "users"."email", "users"."age" FROM "users" AS "users" WHERE "users"."age" > $1`,
		executed[0].SQL)
	assert.Equal(t, []any{18}, executed[0].Args)
}

func TestField_ThroughEmbeddedMixin_ResolvesCorrectly(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "city", "name"},
		Rows:    [][]driver.Value{testdb.Row(int64(1), "Springfield", "Bob")},
	})

	got, err := Of[testCustomer](db).Where(customerCol.City.Eq("Springfield")).Find(context.Background())
	require.NoError(t, err)

	executed := fake.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t,
		`SELECT "customers"."id", "customers"."city", "customers"."name" FROM "customers" AS "customers" WHERE "customers"."city" = $1`,
		executed[0].SQL)
	assert.Equal(t, []any{"Springfield"}, executed[0].Args)

	require.Len(t, got, 1)
	assert.EqualValues(t, 1, got[0].ID)
	assert.Equal(t, "Springfield", got[0].City)
	assert.Equal(t, "Bob", got[0].Name)
}
