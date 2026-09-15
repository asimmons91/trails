package dialect

import (
	"errors"
	"reflect"
)

type ColumnSpec struct {
	PrimaryKey    bool
	AutoIncrement bool
	NotNull       bool
	Unique        bool
	HasDefault    bool
	Default       string
	SQLType       string
}

type ColumnMeta struct {
	Name       string
	SQLType    string
	Nullable   bool
	PrimaryKey bool
}

type DDL interface {
	ColumnDefinition(goType reflect.Type, spec ColumnSpec) (string, error)
	InlinesPrimaryKey(spec ColumnSpec) bool
	AlterColumnSQL(table, column string, goType reflect.Type, spec ColumnSpec) (string, error)
	DropIndexSQL(table, index string) string
	RenameIndexSQL(table, oldName, newName string) (string, error)
	HasTableSQL(table string) (string, []any)
	HasColumnSQL(table, column string) (string, []any)
	HasIndexSQL(table, name string) (string, []any)
	HasConstraintSQL(table, name string) (string, []any)
	GetTablesSQL() (string, []any)
	ColumnTypesSQL(table string) (string, []any)
	CurrentDatabaseSQL() string
}

var ErrRequiresTableRebuild = errors.New("dialect: altering this column requires a table rebuild")
