package sqlbuild

import (
	"reflect"

	"github.com/asimmons91/trails/pack/dialect"
)

type AddColumnBuilder struct {
	table  Table
	column string
	goType reflect.Type
	spec   dialect.ColumnSpec
}

func AddColumn(t Table, column string, goType reflect.Type, spec dialect.ColumnSpec) *AddColumnBuilder {
	return &AddColumnBuilder{table: t, column: column, goType: goType, spec: spec}
}

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

type DropColumnBuilder struct {
	table  Table
	column string
}

func DropColumn(t Table, column string) *DropColumnBuilder {
	return &DropColumnBuilder{table: t, column: column}
}

func (b *DropColumnBuilder) Render(d dialect.Dialect) (string, []any, error) {
	return "ALTER TABLE " + d.QuoteIdent(b.table.Name) + " DROP COLUMN " + d.QuoteIdent(b.column), nil, nil
}

type RenameColumnBuilder struct {
	table            Table
	oldName, newName string
}

func RenameColumn(t Table, oldName, newName string) *RenameColumnBuilder {
	return &RenameColumnBuilder{table: t, oldName: oldName, newName: newName}
}

func (b *RenameColumnBuilder) Render(d dialect.Dialect) (string, []any, error) {
	return "ALTER TABLE " + d.QuoteIdent(b.table.Name) + " RENAME COLUMN " +
		d.QuoteIdent(b.oldName) + " TO " + d.QuoteIdent(b.newName), nil, nil
}

type AlterColumnBuilder struct {
	table  Table
	column string
	goType reflect.Type
	spec   dialect.ColumnSpec
}

func AlterColumn(t Table, column string, goType reflect.Type, spec dialect.ColumnSpec) *AlterColumnBuilder {
	return &AlterColumnBuilder{table: t, column: column, goType: goType, spec: spec}
}

func (b *AlterColumnBuilder) Render(d dialect.Dialect) (string, []any, error) {
	ddl, ok := d.(dialect.DDL)
	if !ok {
		return "", nil, &ErrDDLUnsupportedByDialect{Dialect: d.Name()}
	}

	sqlText, err := ddl.AlterColumnSQL(b.table.Name, b.column, b.goType, b.spec)
	return sqlText, nil, err
}
