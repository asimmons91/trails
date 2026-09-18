package dialect

import (
	"errors"
	"reflect"
)

// ColumnSpec describes one column's constraints and default, independent
// of its SQL type — the input to DDL.ColumnDefinition/AlterColumnSQL.
type ColumnSpec struct {
	PrimaryKey    bool
	AutoIncrement bool
	NotNull       bool
	Unique        bool
	HasDefault    bool
	// Default is the raw SQL default expression, used only if HasDefault.
	Default string
	// SQLType overrides the dialect's inferred SQL type for the column's
	// Go type, from a `type:"..."` struct tag. Empty lets the dialect
	// infer it.
	SQLType string
}

// ColumnMeta describes one column as introspected from an existing table
// (DDL.ColumnTypesSQL's result shape), as opposed to ColumnSpec, which
// describes a column a caller wants to create or alter.
type ColumnMeta struct {
	Name       string
	SQLType    string
	Nullable   bool
	PrimaryKey bool
}

// DDL is the schema-definition half of a dialect, used by pack/migrate to
// generate CREATE/ALTER/DROP statements and introspect existing schema.
type DDL interface {
	// ColumnDefinition renders the column-definition fragment (type plus
	// constraints/default) for a CREATE TABLE or ADD COLUMN statement.
	ColumnDefinition(goType reflect.Type, spec ColumnSpec) (string, error)
	// InlinesPrimaryKey reports whether spec's primary key is written
	// inline in the column definition (e.g. SQLite's
	// "INTEGER PRIMARY KEY AUTOINCREMENT") rather than as a separate
	// table-level constraint.
	InlinesPrimaryKey(spec ColumnSpec) bool
	// AlterColumnSQL renders an ALTER COLUMN statement changing column's
	// type/constraints/default to match spec. It returns
	// ErrRequiresTableRebuild if the dialect can't alter a column in
	// place (SQLite), signaling the caller to fall back to a
	// table-rebuild strategy instead.
	AlterColumnSQL(table, column string, goType reflect.Type, spec ColumnSpec) (string, error)
	// DropIndexSQL renders a statement dropping index.
	DropIndexSQL(table, index string) string
	// RenameIndexSQL renders a statement renaming an index from oldName
	// to newName. It errors if the dialect has no way to rename an index
	// (SQLite).
	RenameIndexSQL(table, oldName, newName string) (string, error)
	// HasTableSQL renders a query returning whether table exists.
	HasTableSQL(table string) (string, []any)
	// HasColumnSQL renders a query returning whether column exists on
	// table.
	HasColumnSQL(table, column string) (string, []any)
	// HasIndexSQL renders a query returning whether an index named name
	// exists on table.
	HasIndexSQL(table, name string) (string, []any)
	// HasConstraintSQL renders a query returning whether a constraint
	// named name exists on table.
	HasConstraintSQL(table, name string) (string, []any)
	// GetTablesSQL renders a query returning every table name in the
	// current database.
	GetTablesSQL() (string, []any)
	// ColumnTypesSQL renders a query returning table's columns as
	// ColumnMeta rows.
	ColumnTypesSQL(table string) (string, []any)
	// CurrentDatabaseSQL renders a query returning the current database's
	// name.
	CurrentDatabaseSQL() string
}

// ErrRequiresTableRebuild is returned by AlterColumnSQL when the dialect
// can't alter a column in place, signaling the caller to fall back to a
// copy/drop/rename table-rebuild strategy (see pack/migrate).
var ErrRequiresTableRebuild = errors.New("dialect: altering this column requires a table rebuild")
