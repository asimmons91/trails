package sqlbuild

import (
	"testing"

	"github.com/asimmons91/trails/pack/dialect"
	"github.com/stretchr/testify/require"
)

// mysqlFake (defined in insert_mysql_seam_test.go) implements dialect.Dialect
// but not dialect.DDL, making it the right fixture to prove every DDL
// builder's type-assertion fallback.
func TestDDLBuilders_UnsupportedByDialect(t *testing.T) {
	cases := []struct {
		name   string
		render func() (string, []any, error)
	}{
		{"CreateTable", func() (string, []any, error) {
			return CreateTable(Table{Name: "widgets"}).
				Column("id", int64Type, dialect.ColumnSpec{}).
				Render(mysqlFake)
		}},
		{"AddColumn", func() (string, []any, error) {
			return AddColumn(Table{Name: "widgets"}, "age", int64Type, dialect.ColumnSpec{}).Render(mysqlFake)
		}},
		{"AlterColumn", func() (string, []any, error) {
			return AlterColumn(Table{Name: "widgets"}, "age", int64Type, dialect.ColumnSpec{}).Render(mysqlFake)
		}},
		{"DropIndex", func() (string, []any, error) {
			return DropIndex(Table{Name: "widgets"}, "idx_age").Render(mysqlFake)
		}},
		{"RenameIndex", func() (string, []any, error) {
			return RenameIndex(Table{Name: "widgets"}, "old_idx", "new_idx").Render(mysqlFake)
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := tc.render()
			require.Error(t, err)
			require.IsType(t, &ErrDDLUnsupportedByDialect{}, err)
		})
	}
}
