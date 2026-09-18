package sqlbuild

import (
	"reflect"

	"github.com/asimmons91/trails/pack/dialect"
)

// AddColumnBuilder renders an ALTER TABLE ... ADD COLUMN statement. Build
// one with AddColumn.
type AddColumnBuilder struct {
	table  Table
	column string
	goType reflect.Type
	spec   dialect.ColumnSpec
}

// AddColumn starts an AddColumnBuilder adding column (of Go type goType,
// with the constraints/default in spec) to t.
func AddColumn(t Table, column string, goType reflect.Type, spec dialect.ColumnSpec) *AddColumnBuilder {
	return &AddColumnBuilder{table: t, column: column, goType: goType, spec: spec}
}

// Render renders b as ALTER TABLE ... ADD COLUMN SQL for dialect d, via
// its dialect.DDL.ColumnDefinition. It errors with
// ErrDDLUnsupportedByDialect if d doesn't implement dialect.DDL.
func (b *AddColumnBuilder) Render(d dialect.Dialect) (string, []any, error) {
	ddl, ok := d.(dialect.DDL)
	if !ok {
		return "", nil, &ErrDDLUnsupportedByDialect{Dialect: d.Name()}
	}

	def, err := ddl.ColumnDefinition(b.goType, b.spec)
	if err != nil {
		return "", nil, err
	}

	return "ALTER TABLE " + d.QuoteIdent(b.table.Name) + " ADD COLUMN " + d.QuoteIdent(b.column) + " " + def, nil, nil
}

// DropColumnBuilder renders an ALTER TABLE ... DROP COLUMN statement.
// Build one with DropColumn.
type DropColumnBuilder struct {
	table  Table
	column string
}

// DropColumn starts a DropColumnBuilder dropping column from t.
func DropColumn(t Table, column string) *DropColumnBuilder {
	return &DropColumnBuilder{table: t, column: column}
}

// Render renders b as ALTER TABLE ... DROP COLUMN SQL for dialect d.
func (b *DropColumnBuilder) Render(d dialect.Dialect) (string, []any, error) {
	return "ALTER TABLE " + d.QuoteIdent(b.table.Name) + " DROP COLUMN " + d.QuoteIdent(b.column), nil, nil
}

// RenameColumnBuilder renders an ALTER TABLE ... RENAME COLUMN statement.
// Build one with RenameColumn.
type RenameColumnBuilder struct {
	table            Table
	oldName, newName string
}

// RenameColumn starts a RenameColumnBuilder renaming t's oldName column to
// newName.
func RenameColumn(t Table, oldName, newName string) *RenameColumnBuilder {
	return &RenameColumnBuilder{table: t, oldName: oldName, newName: newName}
}

// Render renders b as ALTER TABLE ... RENAME COLUMN SQL for dialect d.
func (b *RenameColumnBuilder) Render(d dialect.Dialect) (string, []any, error) {
	return "ALTER TABLE " + d.QuoteIdent(b.table.Name) + " RENAME COLUMN " +
		d.QuoteIdent(b.oldName) + " TO " + d.QuoteIdent(b.newName), nil, nil
}

// AlterColumnBuilder renders a statement changing column's type/
// constraints/default to match spec. Build one with AlterColumn.
type AlterColumnBuilder struct {
	table  Table
	column string
	goType reflect.Type
	spec   dialect.ColumnSpec
}

// AlterColumn starts an AlterColumnBuilder altering column on t to match
// goType/spec.
func AlterColumn(t Table, column string, goType reflect.Type, spec dialect.ColumnSpec) *AlterColumnBuilder {
	return &AlterColumnBuilder{table: t, column: column, goType: goType, spec: spec}
}

// Render renders b via dialect.DDL.AlterColumnSQL, which returns
// dialect.ErrRequiresTableRebuild if d can't alter the column in place
// (SQLite) — see pack/migrate's rebuild fallback. It errors with
// ErrDDLUnsupportedByDialect if d doesn't implement dialect.DDL at all.
func (b *AlterColumnBuilder) Render(d dialect.Dialect) (string, []any, error) {
	ddl, ok := d.(dialect.DDL)
	if !ok {
		return "", nil, &ErrDDLUnsupportedByDialect{Dialect: d.Name()}
	}

	sqlText, err := ddl.AlterColumnSQL(b.table.Name, b.column, b.goType, b.spec)
	return sqlText, nil, err
}
