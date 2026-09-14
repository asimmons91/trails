package pack

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"testing"

	"github.com/asimmons91/trails/pack/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuery_ComparisonPredicates(t *testing.T) {
	cases := []struct {
		name string
		pred Predicate
		want string
		args []any
	}{
		{"Eq", userCol.Age.Eq(18), `"users"."age" = $1`, []any{18}},
		{"Ne", userCol.Age.Ne(18), `"users"."age" <> $1`, []any{18}},
		{"Gt", userCol.Age.Gt(18), `"users"."age" > $1`, []any{18}},
		{"Gte", userCol.Age.Gte(18), `"users"."age" >= $1`, []any{18}},
		{"Lt", userCol.Age.Lt(18), `"users"."age" < $1`, []any{18}},
		{"Lte", userCol.Age.Lte(18), `"users"."age" <= $1`, []any{18}},
		{"In", userCol.Age.In(1, 2, 3), `"users"."age" IN ($1, $2, $3)`, []any{1, 2, 3}},
		{"NotIn", userCol.Age.NotIn(1, 2), `"users"."age" NOT IN ($1, $2)`, []any{1, 2}},
		{"IsNull", userCol.Email.IsNull(), `"users"."email" IS NULL`, []any{}},
		{"IsNotNull", userCol.Email.IsNotNull(), `"users"."email" IS NOT NULL`, []any{}},
		{"Between", userCol.Age.Between(18, 65), `"users"."age" BETWEEN $1 AND $2`, []any{18, 65}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sql, args := executedWhere(t, tc.pred)
			assert.Equal(t,
				`SELECT "users"."id", "users"."email", "users"."age" FROM "users" AS "users" WHERE `+tc.want,
				sql)
			assert.Equal(t, tc.args, args)
		})
	}
}

func TestQuery_Where_ChainedCalls_AreAndJoined(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"id", "email", "age"}})

	_, err := Of[testUser](db).
		Where(userCol.Email.Eq("a@b.com")).
		Where(userCol.Age.Gt(18)).
		Find(context.Background())
	require.NoError(t, err)

	assert.Equal(t,
		`SELECT "users"."id", "users"."email", "users"."age" FROM "users" AS "users" WHERE "users"."email" = $1 AND "users"."age" > $2`,
		fake.Executed()[0].SQL)
}

func TestQuery_WhereOrWhere_ChainingSemantics(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"id", "email", "age"}})

	_, err := Of[testUser](db).
		Where(userCol.Age.Lt(18)).
		Or(userCol.Age.Gt(65)).
		Where(userCol.Email.Eq("a@b.com")).
		Find(context.Background())
	require.NoError(t, err)

	executed := fake.Executed()[0]
	assert.Equal(t,
		`SELECT "users"."id", "users"."email", "users"."age" FROM "users" AS "users" WHERE ("users"."age" < $1 OR "users"."age" > $2) AND "users"."email" = $3`,
		executed.SQL)
	assert.Equal(t, []any{18, 65, "a@b.com"}, executed.Args)
}

func TestQuery_OrderBy_AscDesc(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"id", "email", "age"}})

	_, err := Of[testUser](db).
		Order(userCol.Email.Asc(), userCol.Age.Desc()).
		Find(context.Background())
	require.NoError(t, err)

	assert.Equal(t,
		`SELECT "users"."id", "users"."email", "users"."age" FROM "users" AS "users" ORDER BY "users"."email" ASC, "users"."age" DESC`,
		fake.Executed()[0].SQL)
}

func TestQuery_OrderRaw(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"id", "email", "age"}})

	_, err := Of[testUser](db).OrderRaw("random()").Find(context.Background())
	require.NoError(t, err)

	assert.Equal(t,
		`SELECT "users"."id", "users"."email", "users"."age" FROM "users" AS "users" ORDER BY random()`,
		fake.Executed()[0].SQL)
}

func TestQuery_LimitOffset(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"id", "email", "age"}})

	_, err := Of[testUser](db).Limit(10).Offset(20).Find(context.Background())
	require.NoError(t, err)

	assert.Equal(t,
		`SELECT "users"."id", "users"."email", "users"."age" FROM "users" AS "users" LIMIT 10 OFFSET 20`,
		fake.Executed()[0].SQL)
}

func TestQuery_GroupByHaving(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"id", "email", "age"}})

	_, err := Of[testUser](db).
		GroupBy(userCol.Age).
		Having(userCol.Age.Gt(10)).
		Find(context.Background())
	require.NoError(t, err)

	assert.Equal(t,
		`SELECT "users"."id", "users"."email", "users"."age" FROM "users" AS "users" GROUP BY "users"."age" HAVING "users"."age" > $1`,
		fake.Executed()[0].SQL)
}

func TestQuery_Distinct(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"id", "email", "age"}})

	_, err := Of[testUser](db).Distinct().Find(context.Background())
	require.NoError(t, err)

	assert.Equal(t,
		`SELECT DISTINCT "users"."id", "users"."email", "users"."age" FROM "users" AS "users"`,
		fake.Executed()[0].SQL)
}

func TestQuery_Select_RestrictsColumns(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"email"}})

	_, err := Of[testUser](db).Select(userCol.Email).Find(context.Background())
	require.NoError(t, err)

	assert.Equal(t, `SELECT "users"."email" FROM "users" AS "users"`, fake.Executed()[0].SQL)
}

func TestQuery_Select_Default_IsFullFieldListInDeclarationOrder(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"id", "email", "age"}})

	_, err := Of[testUser](db).Find(context.Background())
	require.NoError(t, err)

	assert.Equal(t,
		`SELECT "users"."id", "users"."email", "users"."age" FROM "users" AS "users"`,
		fake.Executed()[0].SQL)
}

func TestQuery_ForUpdate(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"id", "email", "age"}})

	_, err := Of[testUser](db).ForUpdate().Find(context.Background())
	require.NoError(t, err)

	assert.Equal(t,
		`SELECT "users"."id", "users"."email", "users"."age" FROM "users" AS "users" FOR UPDATE`,
		fake.Executed()[0].SQL)
}

func TestQuery_ForShare(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"id", "email", "age"}})

	_, err := Of[testUser](db).ForShare().Find(context.Background())
	require.NoError(t, err)

	assert.Equal(t,
		`SELECT "users"."id", "users"."email", "users"."age" FROM "users" AS "users" FOR SHARE`,
		fake.Executed()[0].SQL)
}

func TestQuery_ForUpdateAndForShare_IsError(t *testing.T) {
	db, fake := newTestDB()

	_, err := Of[testUser](db).ForUpdate().ForShare().Find(context.Background())
	require.Error(t, err)
	assert.Empty(t, fake.Executed(), "should fail to render before reaching the database")
}

func TestQuery_WhereRaw(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"id", "email", "age"}})

	_, err := Of[testUser](db).WhereRaw(`"age" > $1`, 18).Find(context.Background())
	require.NoError(t, err)

	executed := fake.Executed()[0]
	assert.Equal(t,
		`SELECT "users"."id", "users"."email", "users"."age" FROM "users" AS "users" WHERE "age" > $1`,
		executed.SQL)
	assert.Equal(t, []any{18}, executed.Args)
}

func TestQuery_SelectRaw(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"id", "email", "age", "cnt"}})

	_, err := Of[testUser](db).SelectRaw("count(*) OVER ()").Find(context.Background())
	require.NoError(t, err)

	assert.Equal(t,
		`SELECT "users"."id", "users"."email", "users"."age", count(*) OVER () FROM "users" AS "users"`,
		fake.Executed()[0].SQL)
}

func TestQuery_Find_ReturnsAllRows(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "email", "age"},
		Rows: [][]driver.Value{
			testdb.Row(int64(1), "a@b.com", int64(30)),
			testdb.Row(int64(2), "c@d.com", int64(40)),
		},
	})

	got, err := Of[testUser](db).Find(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []testUser{
		{ID: 1, Email: "a@b.com", Age: 30},
		{ID: 2, Email: "c@d.com", Age: 40},
	}, got)
}

func TestQuery_First_ReturnsFirstRowAndLimitsToOne(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"id", "email", "age"},
		Rows:    [][]driver.Value{testdb.Row(int64(1), "a@b.com", int64(30))},
	})

	got, err := Of[testUser](db).First(context.Background())
	require.NoError(t, err)
	assert.Equal(t, testUser{ID: 1, Email: "a@b.com", Age: 30}, got)
	assert.Equal(t,
		`SELECT "users"."id", "users"."email", "users"."age" FROM "users" AS "users" LIMIT 1`,
		fake.Executed()[0].SQL)
}

func TestQuery_First_NoRows_ReturnsErrNoRows(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"id", "email", "age"}})

	_, err := Of[testUser](db).First(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNoRows)
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestQuery_Count(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"count"},
		Rows:    [][]driver.Value{testdb.Row(int64(5))},
	})

	n, err := Of[testUser](db).Where(userCol.Age.Gt(18)).Count(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, 5, n)
	assert.Equal(t,
		`SELECT count(*) FROM "users" AS "users" WHERE "users"."age" > $1`,
		fake.Executed()[0].SQL)
}

func TestQuery_Exists_True(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{
		Columns: []string{"exists_col"},
		Rows:    [][]driver.Value{testdb.Row(int64(1))},
	})

	ok, err := Of[testUser](db).Where(userCol.Age.Gt(18)).Exists(context.Background())
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t,
		`SELECT 1 FROM "users" AS "users" WHERE "users"."age" > $1 LIMIT 1`,
		fake.Executed()[0].SQL)
}

func TestQuery_Exists_False(t *testing.T) {
	db, fake := newTestDB()
	fake.Enqueue(testdb.Result{Columns: []string{"exists_col"}})

	ok, err := Of[testUser](db).Exists(context.Background())
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestQuery_SafeToBuildOnTwice(t *testing.T) {
	db, fake := newTestDB()
	base := Of[testUser](db).Where(userCol.Age.Gt(18))

	fake.Enqueue(testdb.Result{Columns: []string{"id", "email", "age"}})
	_, err := base.Order(userCol.Email.Asc()).Find(context.Background())
	require.NoError(t, err)
	withOrderSQL := fake.Executed()[0].SQL

	fake.Reset()
	fake.Enqueue(testdb.Result{
		Columns: []string{"count"},
		Rows:    [][]driver.Value{testdb.Row(int64(3))},
	})
	n, err := base.Count(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, 3, n)

	assert.Equal(t,
		`SELECT "users"."id", "users"."email", "users"."age" FROM "users" AS "users" WHERE "users"."age" > $1 ORDER BY "users"."email" ASC`,
		withOrderSQL)
	assert.Equal(t,
		`SELECT count(*) FROM "users" AS "users" WHERE "users"."age" > $1`,
		fake.Executed()[0].SQL)
}
