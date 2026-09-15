package sqlbuild

import (
	"errors"
	"testing"

	"github.com/asimmons91/trails/pack/dialect/sqlitedialect"
	"github.com/stretchr/testify/require"
)

func TestCreateIndex_PerDialect(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `CREATE INDEX "idx_widgets_name" ON "widgets" ("name")`},
		{"MySQL", mysql, "CREATE INDEX `idx_widgets_name` ON `widgets` (`name`)"},
		{"SQLite", sqlite, `CREATE INDEX "idx_widgets_name" ON "widgets" ("name")`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, _, err := CreateIndex(Table{Name: "widgets"}, "idx_widgets_name", "name").Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
		})
	}
}

func TestCreateIndex_Unique_MultiColumn(t *testing.T) {
	sql, _, err := CreateIndex(Table{Name: "widgets"}, "idx_widgets_a_b", "a", "b").Unique().Render(pg)
	require.NoError(t, err)
	require.Equal(t, `CREATE UNIQUE INDEX "idx_widgets_a_b" ON "widgets" ("a", "b")`, sql)
}

func TestDropIndex_PerDialect(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `DROP INDEX "idx_widgets_name"`},
		{"MySQL", mysql, "DROP INDEX `idx_widgets_name` ON `widgets`"},
		{"SQLite", sqlite, `DROP INDEX "idx_widgets_name"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, _, err := DropIndex(Table{Name: "widgets"}, "idx_widgets_name").Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
		})
	}
}

func TestRenameIndex_PerDialect(t *testing.T) {
	for _, tc := range []dialectCase{
		{"Postgres", pg, `ALTER INDEX "old_idx" RENAME TO "new_idx"`},
		{"MySQL", mysql, "ALTER TABLE `widgets` RENAME INDEX `old_idx` TO `new_idx`"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, _, err := RenameIndex(Table{Name: "widgets"}, "old_idx", "new_idx").Render(tc.d)
			require.NoError(t, err)
			require.Equal(t, tc.wantSQL, sql)
		})
	}
}

func TestRenameIndex_SQLite_Unsupported(t *testing.T) {
	_, _, err := RenameIndex(Table{Name: "widgets"}, "old_idx", "new_idx").Render(sqlite)
	require.Error(t, err)
	require.True(t, errors.Is(err, sqlitedialect.ErrRenameIndexUnsupported))
}
