package sqlbuild

import (
	"reflect"
	"testing"
	"time"

	"github.com/asimmons91/trails/pack/dialect"
	"github.com/stretchr/testify/require"
)

var (
	int64Type = reflect.TypeFor[int64]()
	stringT   = reflect.TypeFor[string]()
	timeT     = reflect.TypeFor[time.Time]()
)

func TestCreateTable_AutoIncrementPK_PerDialect(t *testing.T) {
	build := func(d dialect.Dialect) (string, []any, error) {
		return CreateTable(Table{Name: "widgets"}).
			Column("id", int64Type, dialect.ColumnSpec{PrimaryKey: true, AutoIncrement: true}).
			Column("name", stringT, dialect.ColumnSpec{NotNull: true}).
			Column("email", stringT, dialect.ColumnSpec{Unique: true}).
			Column("created_at", timeT, dialect.ColumnSpec{HasDefault: true, Default: "now()"}).
			Render(d)
	}

	for _, tc := range []dialectCase{
		{"Postgres", pg, `CREATE TABLE "widgets" ("id" BIGSERIAL, "name" TEXT NOT NULL, "email" TEXT UNIQUE, "created_at" TIMESTAMPTZ DEFAULT now(), PRIMARY KEY ("id"))`},
		{"MySQL", mysql, "CREATE TABLE `widgets` (`id` BIGINT AUTO_INCREMENT, `name` VARCHAR(255) NOT NULL, `email` VARCHAR(255) UNIQUE, `created_at` DATETIME DEFAULT now(), PRIMARY KEY (`id`))"},
		{"SQLite", sqlite, `CREATE TABLE "widgets" ("id" INTEGER PRIMARY KEY AUTOINCREMENT, "name" TEXT NOT NULL, "email" TEXT UNIQUE, "created_at" TEXT DEFAULT now())`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, _, err := build(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
		})
	}
}

func TestCreateTable_CompositePrimaryKey_PerDialect(t *testing.T) {
	build := func(d dialect.Dialect) (string, []any, error) {
		return CreateTable(Table{Name: "memberships"}).
			Column("user_id", int64Type, dialect.ColumnSpec{PrimaryKey: true}).
			Column("group_id", int64Type, dialect.ColumnSpec{PrimaryKey: true}).
			Render(d)
	}

	for _, tc := range []dialectCase{
		{"Postgres", pg, `CREATE TABLE "memberships" ("user_id" BIGINT, "group_id" BIGINT, PRIMARY KEY ("user_id", "group_id"))`},
		{"MySQL", mysql, "CREATE TABLE `memberships` (`user_id` BIGINT, `group_id` BIGINT, PRIMARY KEY (`user_id`, `group_id`))"},
		{"SQLite", sqlite, `CREATE TABLE "memberships" ("user_id" INTEGER, "group_id" INTEGER, PRIMARY KEY ("user_id", "group_id"))`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, _, err := build(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
		})
	}
}

func TestCreateTable_IfNotExists(t *testing.T) {
	sql, _, err := CreateTable(Table{Name: "widgets"}).
		IfNotExists().
		Column("id", int64Type, dialect.ColumnSpec{PrimaryKey: true}).
		Render(pg)
	require.NoError(t, err)
	require.Equal(t, `CREATE TABLE IF NOT EXISTS "widgets" ("id" BIGINT, PRIMARY KEY ("id"))`, sql)
}

func TestCreateTable_UnmappedGoType_WithoutSQLTypeTag_Errors(t *testing.T) {
	type unmapped struct{ V chan int }
	chanType := reflect.TypeFor[unmapped]().Field(0).Type

	_, _, err := CreateTable(Table{Name: "widgets"}).
		Column("v", chanType, dialect.ColumnSpec{}).
		Render(pg)
	require.Error(t, err)
}

func TestCreateTable_ExplicitSQLType_OverridesGoTypeMapping(t *testing.T) {
	sql, _, err := CreateTable(Table{Name: "widgets"}).
		Column("total", int64Type, dialect.ColumnSpec{SQLType: "numeric(10,2)"}).
		Render(pg)
	require.NoError(t, err)
	require.Equal(t, `CREATE TABLE "widgets" ("total" numeric(10,2))`, sql)
}
