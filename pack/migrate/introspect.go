package migrate

import (
	"context"

	"github.com/asimmons91/trails/pack/dialect"
)

type ColumnType = dialect.ColumnMeta

func (m *Migrator) GetTables(ctx context.Context) ([]string, error) {
	sqlText, args := m.ddl.GetTablesSQL()

	rows, err := m.db.QueryContext(ctx, "GetTables", "", sqlText, args)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}

	return out, rows.Err()
}

func (m *Migrator) ColumnTypes(ctx context.Context, dst any) ([]ColumnType, error) {
	table := schemaForOrPanic(dst)
	sqlText, args := m.ddl.ColumnTypesSQL(table.Name)

	rows, err := m.db.QueryContext(ctx, "ColumnTypes", table.GoType.Name(), sqlText, args)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []ColumnType
	for rows.Next() {
		var ct ColumnType
		if err := rows.Scan(&ct.Name, &ct.SQLType, &ct.Nullable, &ct.PrimaryKey); err != nil {
			return nil, err
		}
		out = append(out, ct)
	}

	return out, rows.Err()
}

func (m *Migrator) CurrentDatabase(ctx context.Context) (string, error) {
	sqlText := m.ddl.CurrentDatabaseSQL()

	rows, err := m.db.QueryContext(ctx, "CurrentDatabase", "", sqlText, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return "", err
		}
		return "", nil
	}

	var name string
	if err := rows.Scan(&name); err != nil {
		return "", err
	}

	return name, rows.Err()
}
