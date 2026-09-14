package migrate

import (
	"context"

	"github.com/asimmons91/trails/pack/internal/schema"
	"github.com/asimmons91/trails/pack/internal/sqlbuild"
)

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

func (m *Migrator) HasColumn(ctx context.Context, dst any, field string) (bool, error) {
	table := schemaForOrPanic(dst)
	column := resolveColumnName(table, field)
	return m.hasColumnByName(ctx, table, column)
}

func (m *Migrator) hasColumnByName(ctx context.Context, table *schema.Table, column string) (bool, error) {
	sqlText, args := m.ddl.HasColumnSQL(table.Name, column)
	return m.queryExists(ctx, "HasColumn", table.GoType.Name(), sqlText, args)
}

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
