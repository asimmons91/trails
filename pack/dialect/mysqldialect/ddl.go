package mysqldialect

import (
	"fmt"
	"reflect"
	"time"

	"github.com/asimmons91/trails/pack/dialect"
)

var _ dialect.DDL = MySQL{}

var timeType = reflect.TypeFor[time.Time]()

func (MySQL) baseType(t reflect.Type) (string, error) {
	switch t.Kind() {
	case reflect.Bool:
		return "TINYINT(1)", nil
	case reflect.Int8, reflect.Int16, reflect.Int32,
		reflect.Uint8, reflect.Uint16, reflect.Uint32:
		return "INT", nil
	case reflect.Int, reflect.Int64, reflect.Uint, reflect.Uint64:
		return "BIGINT", nil
	case reflect.String:
		return "VARCHAR(255)", nil
	case reflect.Float32:
		return "FLOAT", nil
	case reflect.Float64:
		return "DOUBLE", nil
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			return "BLOB", nil
		}
	}

	if t == timeType {
		return "DATETIME", nil
	}

	return "", fmt.Errorf("mysqldialect: no SQL type mapping for Go type %s; set a `type:` struct tag", t)
}

func (d MySQL) ColumnDefinition(t reflect.Type, spec dialect.ColumnSpec) (string, error) {
	sqlType := spec.SQLType
	if sqlType == "" {
		bt, err := d.baseType(t)
		if err != nil {
			return "", err
		}
		sqlType = bt
	}

	def := sqlType
	if spec.NotNull {
		def += " NOT NULL"
	}
	if spec.AutoIncrement {
		def += " AUTO_INCREMENT"
	}
	if spec.Unique {
		def += " UNIQUE"
	}
	if spec.HasDefault {
		def += " DEFAULT " + spec.Default
	}

	return def, nil
}

func (MySQL) InlinesPrimaryKey(dialect.ColumnSpec) bool { return false }

func (d MySQL) AlterColumnSQL(table, column string, t reflect.Type, spec dialect.ColumnSpec) (string, error) {
	def, err := d.ColumnDefinition(t, spec)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s %s", d.QuoteIdent(table), d.QuoteIdent(column), def), nil
}

func (d MySQL) DropIndexSQL(table, index string) string {
	return "DROP INDEX " + d.QuoteIdent(index) + " ON " + d.QuoteIdent(table)
}

func (d MySQL) RenameIndexSQL(table, oldName, newName string) (string, error) {
	return fmt.Sprintf("ALTER TABLE %s RENAME INDEX %s TO %s", d.QuoteIdent(table), d.QuoteIdent(oldName), d.QuoteIdent(newName)), nil
}

func (MySQL) HasTableSQL(table string) (string, []any) {
	return "SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?)", []any{table}
}

func (MySQL) HasColumnSQL(table, column string) (string, []any) {
	return "SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?)", []any{table, column}
}

func (MySQL) HasIndexSQL(table, index string) (string, []any) {
	return "SELECT EXISTS (SELECT 1 FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?)", []any{table, index}
}

func (MySQL) HasConstraintSQL(table, name string) (string, []any) {
	return "SELECT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE table_schema = DATABASE() AND table_name = ? AND constraint_name = ?)", []any{table, name}
}

func (MySQL) GetTablesSQL() (string, []any) {
	return "SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE() ORDER BY table_name", nil
}

func (MySQL) ColumnTypesSQL(table string) (string, []any) {
	return `SELECT column_name, data_type, (is_nullable = 'YES') AS nullable, (column_key = 'PRI') AS is_primary_key
		FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name = ?
		ORDER BY ordinal_position`, []any{table}
}

func (MySQL) CurrentDatabaseSQL() string { return "SELECT DATABASE()" }
