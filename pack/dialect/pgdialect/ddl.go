package pgdialect

import (
	"fmt"
	"reflect"
	"time"

	"github.com/asimmons91/trails/pack/dialect"
)

var _ dialect.DDL = Postgres{}

var timeType = reflect.TypeFor[time.Time]()

func (Postgres) baseType(t reflect.Type, autoIncrement bool) (string, error) {
	switch t.Kind() {
	case reflect.Bool:
		return "BOOLEAN", nil
	case reflect.Int8, reflect.Int16, reflect.Int32,
		reflect.Uint8, reflect.Uint16, reflect.Uint32:
		if autoIncrement {
			return "SERIAL", nil
		}
		return "INTEGER", nil
	case reflect.Int, reflect.Int64, reflect.Uint, reflect.Uint64:
		if autoIncrement {
			return "BIGSERIAL", nil
		}
		return "BIGINT", nil
	case reflect.String:
		return "TEXT", nil
	case reflect.Float32:
		return "REAL", nil
	case reflect.Float64:
		return "DOUBLE PRECISION", nil
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			return "BYTEA", nil
		}
	}

	if t == timeType {
		return "TIMESTAMPTZ", nil
	}

	return "", fmt.Errorf("pgdialect: no SQL type mapping for Go type %s; set a `type:` struct tag", t)
}

// ColumnDefinition implements dialect.DDL.
func (d Postgres) ColumnDefinition(t reflect.Type, spec dialect.ColumnSpec) (string, error) {
	sqlType := spec.SQLType
	if sqlType == "" {
		bt, err := d.baseType(t, spec.AutoIncrement)
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

func (Postgres) InlinesPrimaryKey(dialect.ColumnSpec) bool { return false }

func (d Postgres) AlterColumnSQL(table, column string, t reflect.Type, spec dialect.ColumnSpec) (string, error) {
	sqlType := spec.SQLType
	if sqlType == "" {
		bt, err := d.baseType(t, spec.AutoIncrement)
		if err != nil {
			return "", err
		}
		sqlType = bt
	}

	qCol := d.QuoteIdent(column)
	actions := []string{
		fmt.Sprintf("ALTER COLUMN %s TYPE %s", qCol, sqlType),
	}
	if spec.NotNull {
		actions = append(actions, fmt.Sprintf("ALTER COLUMN %s SET NOT NULL", qCol))
	} else {
		actions = append(actions, fmt.Sprintf("ALTER COLUMN %s DROP NOT NULL", qCol))
	}
	if spec.HasDefault {
		actions = append(actions, fmt.Sprintf("ALTER COLUMN %s SET DEFAULT %s", qCol, spec.Default))
	} else {
		actions = append(actions, fmt.Sprintf("ALTER COLUMN %s DROP DEFAULT", qCol))
	}

	sql := "ALTER TABLE " + d.QuoteIdent(table) + " "
	for i, a := range actions {
		if i > 0 {
			sql += ", "
		}
		sql += a
	}

	return sql, nil
}

func (d Postgres) DropIndexSQL(_, index string) string {
	return "DROP INDEX " + d.QuoteIdent(index)
}

func (d Postgres) RenameIndexSQL(_, oldName, newName string) (string, error) {
	return "ALTER INDEX " + d.QuoteIdent(oldName) + " RENAME TO " + d.QuoteIdent(newName), nil
}

func (Postgres) HasTableSQL(table string) (string, []any) {
	return "SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = $1)", []any{table}
}

func (Postgres) HasColumnSQL(table, column string) (string, []any) {
	return "SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2)", []any{table, column}
}

func (Postgres) HasIndexSQL(table, index string) (string, []any) {
	return "SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE schemaname = current_schema() AND tablename = $1 AND indexname = $2)", []any{table, index}
}

func (Postgres) HasConstraintSQL(table, name string) (string, []any) {
	return "SELECT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE table_schema = current_schema() AND table_name = $1 AND constraint_name = $2)", []any{table, name}
}

func (Postgres) GetTablesSQL() (string, []any) {
	return "SELECT table_name FROM information_schema.tables WHERE table_schema = current_schema() ORDER BY table_name", nil
}

func (Postgres) ColumnTypesSQL(table string) (string, []any) {
	return `SELECT c.column_name, c.data_type, (c.is_nullable = 'YES') AS nullable,
		EXISTS (
			SELECT 1 FROM information_schema.table_constraints tc
			JOIN information_schema.key_column_usage kcu
				ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema
			WHERE tc.table_schema = current_schema() AND tc.table_name = c.table_name
				AND tc.constraint_type = 'PRIMARY KEY' AND kcu.column_name = c.column_name
		) AS is_primary_key
		FROM information_schema.columns c
		WHERE c.table_schema = current_schema() AND c.table_name = $1
		ORDER BY c.ordinal_position`, []any{table}
}

func (Postgres) CurrentDatabaseSQL() string { return "SELECT current_database()" }
