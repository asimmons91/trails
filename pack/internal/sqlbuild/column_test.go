package sqlbuild

import (
	"errors"
	"testing"

	"github.com/asimmons91/trails/pack/dialect"
	"github.com/stretchr/testify/require"
)

func TestAddColumn_PerDialect(t *testing.T) {
	build := func(d dialect.Dialect) (string, []any, error) {
		return AddColumn(Table{Name: "widgets"}, "age", int64Type, dialect.ColumnSpec{NotNull: true}).Render(d)
	}

	for _, tc := range []dialectCase{
		{"Postgres", pg, `ALTER TABLE "widgets" ADD COLUMN "age" BIGINT NOT NULL`},
		{"MySQL", mysql, "ALTER TABLE `widgets` ADD COLUMN `age` BIGINT NOT NULL"},
		{"SQLite", sqlite, `ALTER TABLE "widgets" ADD COLUMN "age" INTEGER NOT NULL`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, _, err := build(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
		})
	}
}

func TestDropColumn_PerDialect(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `ALTER TABLE "widgets" DROP COLUMN "age"`},
		{"MySQL", mysql, "ALTER TABLE `widgets` DROP COLUMN `age`"},
		{"SQLite", sqlite, `ALTER TABLE "widgets" DROP COLUMN "age"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, _, err := DropColumn(Table{Name: "widgets"}, "age").Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
		})
	}
}

func TestRenameColumn_PerDialect(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `ALTER TABLE "widgets" RENAME COLUMN "age" TO "years"`},
		{"MySQL", mysql, "ALTER TABLE `widgets` RENAME COLUMN `age` TO `years`"},
		{"SQLite", sqlite, `ALTER TABLE "widgets" RENAME COLUMN "age" TO "years"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, _, err := RenameColumn(Table{Name: "widgets"}, "age", "years").Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
		})
	}
}

func TestAlterColumn_Postgres_SingleMultiClauseStatement(t *testing.T) {
	sql, _, err := AlterColumn(Table{Name: "widgets"}, "age", int64Type, dialect.ColumnSpec{NotNull: true}).Render(pg)
	require.NoError(t, err)
	require.Equal(t, `ALTER TABLE "widgets" ALTER COLUMN "age" TYPE BIGINT, ALTER COLUMN "age" SET NOT NULL, ALTER COLUMN "age" DROP DEFAULT`, sql)
}

func TestAlterColumn_MySQL_ModifyColumn(t *testing.T) {
	sql, _, err := AlterColumn(Table{Name: "widgets"}, "age", int64Type, dialect.ColumnSpec{NotNull: true}).Render(mysql)
	require.NoError(t, err)
	require.Equal(t, "ALTER TABLE `widgets` MODIFY COLUMN `age` BIGINT NOT NULL", sql)
}

func TestAlterColumn_SQLite_ReturnsRequiresTableRebuild(t *testing.T) {
	_, _, err := AlterColumn(Table{Name: "widgets"}, "age", int64Type, dialect.ColumnSpec{NotNull: true}).Render(sqlite)
	require.Error(t, err)
	require.True(t, errors.Is(err, dialect.ErrRequiresTableRebuild))
}
