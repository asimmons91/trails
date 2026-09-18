package migrate

import (
	"context"

	"github.com/asimmons91/trails/pack/internal/schema"
	"github.com/asimmons91/trails/pack/internal/sqlbuild"
)

// AddColumn adds dst's field (a Go struct field name) as a new column,
// deriving its type and spec from the same struct tags CreateTable uses.
func (m *Migrator) AddColumn(ctx context.Context, dst any, field string) error {
	table := schemaForOrPanic(dst)
	f := fieldByGoNameOrPanic(table, field)

	sqlText, args, err := sqlbuild.AddColumn(sqlTable(table), f.Column, f.Type, toColumnSpec(f.Options)).Render(m.db.Dialect())
	if err != nil {
		return err
	}
	_, err = m.db.ExecContext(ctx, "AddColumn", table.GoType.Name(), sqlText, args)
	return err
}

// DropColumn drops field (a Go struct field name or a bare column name)
// from dst's table. It's a no-op if the column doesn't already exist,
// unless opts includes WithoutIfExists.
func (m *Migrator) DropColumn(ctx context.Context, dst any, field string, opts ...DropOption) error {
	table := schemaForOrPanic(dst)
	column := resolveColumnName(table, field)
	cfg := applyDropOptions(opts)

	if !cfg.withoutIfExists {
		exists, err := m.hasColumnByName(ctx, table, column)
		if err != nil {
			return err
		}
		if !exists {
			return nil
		}
	}

	sqlText, args, err := sqlbuild.DropColumn(sqlTable(table), column).Render(m.db.Dialect())
	if err != nil {
		return err
	}
	_, err = m.db.ExecContext(ctx, "DropColumn", table.GoType.Name(), sqlText, args)
	return err
}

// HasColumn reports whether field (a Go struct field name or a bare column
// name) exists as a column on dst's table.
func (m *Migrator) HasColumn(ctx context.Context, dst any, field string) (bool, error) {
	table := schemaForOrPanic(dst)
	column := resolveColumnName(table, field)
	return m.hasColumnByName(ctx, table, column)
}

// hasColumnByName is HasColumn once field has already been resolved to a
// bare column name.
func (m *Migrator) hasColumnByName(ctx context.Context, table *schema.Table, column string) (bool, error) {
	sqlText, args := m.ddl.HasColumnSQL(table.Name, column)
	return m.queryExists(ctx, "HasColumn", table.GoType.Name(), sqlText, args)
}

// RenameColumn renames field (a Go struct field name or a bare column
// name) on dst's table to newName.
func (m *Migrator) RenameColumn(ctx context.Context, dst any, field, newName string) error {
	table := schemaForOrPanic(dst)
	column := resolveColumnName(table, field)

	sqlText, args, err := sqlbuild.RenameColumn(sqlTable(table), column, newName).Render(m.db.Dialect())
	if err != nil {
		return err
	}
	_, err = m.db.ExecContext(ctx, "RenameColumn", table.GoType.Name(), sqlText, args)
	return err
}
