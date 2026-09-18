package migrate

import (
	"context"

	"github.com/asimmons91/trails/pack/internal/sqlbuild"
)

// CreateIndex creates an index named name on dst's table, covering fields
// (Go struct field names or bare column names, in order). Pass
// WithUniqueIndex to create it as UNIQUE.
func (m *Migrator) CreateIndex(ctx context.Context, dst any, name string, fields []string, opts ...IndexOption) error {
	table := schemaForOrPanic(dst)
	cfg := applyIndexOptions(opts)

	cols := make([]string, len(fields))
	for i, f := range fields {
		cols[i] = resolveColumnName(table, f)
	}

	b := sqlbuild.CreateIndex(sqlTable(table), name, cols...)
	if cfg.unique {
		b = b.Unique()
	}

	sqlText, args, err := b.Render(m.db.Dialect())
	if err != nil {
		return err
	}
	_, err = m.db.ExecContext(ctx, "CreateIndex", table.GoType.Name(), sqlText, args)
	return err
}

// DropIndex drops the named index from dst's table. It's a no-op if the
// index doesn't already exist, unless opts includes WithoutIfExists.
func (m *Migrator) DropIndex(ctx context.Context, dst any, name string, opts ...DropOption) error {
	table := schemaForOrPanic(dst)
	cfg := applyDropOptions(opts)

	if !cfg.withoutIfExists {
		exists, err := m.HasIndex(ctx, dst, name)
		if err != nil {
			return err
		}
		if !exists {
			return nil
		}
	}

	sqlText, args, err := sqlbuild.DropIndex(sqlTable(table), name).Render(m.db.Dialect())
	if err != nil {
		return err
	}
	_, err = m.db.ExecContext(ctx, "DropIndex", table.GoType.Name(), sqlText, args)
	return err
}

// HasIndex reports whether the named index exists on dst's table.
func (m *Migrator) HasIndex(ctx context.Context, dst any, name string) (bool, error) {
	table := schemaForOrPanic(dst)
	sqlText, args := m.ddl.HasIndexSQL(table.Name, name)
	return m.queryExists(ctx, "HasIndex", table.GoType.Name(), sqlText, args)
}

// RenameIndex renames the index oldName on dst's table to newName. SQLite
// has no native rename-index operation, so this always fails on it (see
// sqlitedialect.ErrRenameIndexUnsupported).
func (m *Migrator) RenameIndex(ctx context.Context, dst any, oldName, newName string) error {
	table := schemaForOrPanic(dst)

	sqlText, args, err := sqlbuild.RenameIndex(sqlTable(table), oldName, newName).Render(m.db.Dialect())
	if err != nil {
		return err
	}
	_, err = m.db.ExecContext(ctx, "RenameIndex", table.GoType.Name(), sqlText, args)
	return err
}
