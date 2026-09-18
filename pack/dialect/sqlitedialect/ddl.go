package sqlitedialect

import (
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/asimmons91/trails/pack/dialect"
)

var _ dialect.DDL = SQLite{}

var timeType = reflect.TypeFor[time.Time]()

// ErrRenameIndexUnsupported is returned by RenameIndexSQL: SQLite has no
// RENAME INDEX statement and no fallback, unlike AlterColumnSQL's
// table-rebuild path.
var ErrRenameIndexUnsupported = errors.New("sqlitedialect: RENAME INDEX is not supported by SQLite")

func isIntegerKind(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	}
	return false
}

func (s SQLite) baseType(t reflect.Type) (string, error) {
	switch {
	case t.Kind() == reflect.Bool:
		return "INTEGER", nil
	case isIntegerKind(t.Kind()):
		return "INTEGER", nil
	case t.Kind() == reflect.String:
		return "TEXT", nil
	case t.Kind() == reflect.Float32, t.Kind() == reflect.Float64:
		return "REAL", nil
	case t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8:
		return "BLOB", nil
	case t == timeType:
		return "TEXT", nil
	}

	return "", fmt.Errorf("sqlitedialect: no SQL type mapping for Go type %s; set a `type:` struct tag", t)
}

// ColumnDefinition implements dialect.DDL.
func (s SQLite) ColumnDefinition(t reflect.Type, spec dialect.ColumnSpec) (string, error) {
	if spec.PrimaryKey && spec.AutoIncrement {
		if !isIntegerKind(t.Kind()) {
			return "", fmt.Errorf("sqlitedialect: auto_increment primary key must be an integer Go type, got %s", t)
		}
		return "INTEGER PRIMARY KEY AUTOINCREMENT", nil
	}

	sqlType := spec.SQLType
	if sqlType == "" {
		bt, err := s.baseType(t)
		if err != nil {
			return "", err
		}
		sqlType = bt
	}

	def := sqlType
	if spec.NotNull {
		def += " NOT NULL"
	}
	if spec.Unique {
		def += " UNIQUE"
	}
	if spec.HasDefault {
		def += " DEFAULT " + spec.Default
	}

	return def, nil
}

// InlinesPrimaryKey is true only for an auto-increment primary key, which
// SQLite requires to be written inline as "INTEGER PRIMARY KEY
// AUTOINCREMENT"; any other primary key is a separate table constraint.
func (SQLite) InlinesPrimaryKey(spec dialect.ColumnSpec) bool {
	return spec.PrimaryKey && spec.AutoIncrement
}

// AlterColumnSQL always returns dialect.ErrRequiresTableRebuild: SQLite has
// no ALTER COLUMN, so callers (pack/migrate) must fall back to a
// copy/drop/rename table rebuild instead.
func (SQLite) AlterColumnSQL(string, string, reflect.Type, dialect.ColumnSpec) (string, error) {
	return "", dialect.ErrRequiresTableRebuild
}

// DropIndexSQL implements dialect.DDL.
func (s SQLite) DropIndexSQL(_, index string) string {
	return "DROP INDEX " + s.QuoteIdent(index)
}

// RenameIndexSQL always returns ErrRenameIndexUnsupported: SQLite has no
// RENAME INDEX statement.
func (SQLite) RenameIndexSQL(string, string, string) (string, error) {
	return "", ErrRenameIndexUnsupported
}

// HasTableSQL implements dialect.DDL.
func (s SQLite) HasTableSQL(table string) (string, []any) {
	return "SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = " + s.Placeholder(1) + ")", []any{table}
}

// HasColumnSQL implements dialect.DDL.
func (s SQLite) HasColumnSQL(table, column string) (string, []any) {
	return "SELECT EXISTS (SELECT 1 FROM pragma_table_info(" + s.Placeholder(1) + ") WHERE name = " + s.Placeholder(2) + ")", []any{table, column}
}

// HasIndexSQL implements dialect.DDL.
func (s SQLite) HasIndexSQL(_, index string) (string, []any) {
	return "SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type = 'index' AND name = " + s.Placeholder(1) + ")", []any{index}
}

// HasConstraintSQL implements dialect.DDL, matching SQLite's
// pack-generated foreign-key constraint naming convention
// ("fk_<table>_<name>") against pragma_foreign_key_list's "from" column.
func (d SQLite) HasConstraintSQL(table, name string) (string, []any) {
	return "SELECT EXISTS (SELECT 1 FROM pragma_foreign_key_list(" + d.Placeholder(1) + ") WHERE " +
		d.Placeholder(2) + " = 'fk_' || " + d.Placeholder(3) + " || '_' || \"from\")", []any{table, name, table}
}

// GetTablesSQL implements dialect.DDL, excluding SQLite's own
// "sqlite_%"-prefixed internal tables.
func (SQLite) GetTablesSQL() (string, []any) {
	return "SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name", nil
}

// ColumnTypesSQL implements dialect.DDL.
func (s SQLite) ColumnTypesSQL(table string) (string, []any) {
	return "SELECT name, type, (\"notnull\" = 0) AS nullable, (pk > 0) AS is_primary_key FROM pragma_table_info(" + s.Placeholder(1) + ") ORDER BY cid", []any{table}
}

// CurrentDatabaseSQL returns the file path of SQLite's "main" database.
func (SQLite) CurrentDatabaseSQL() string {
	return "SELECT file FROM pragma_database_list WHERE name = 'main'"
}
