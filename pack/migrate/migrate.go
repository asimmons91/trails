package migrate

import (
	"context"
	"fmt"

	"github.com/asimmons91/trails/pack"
	"github.com/asimmons91/trails/pack/dialect"
	"github.com/asimmons91/trails/pack/internal/schema"
	"github.com/asimmons91/trails/pack/internal/sqlbuild"
)

type Migrator struct {
	db  *pack.DB
	ddl dialect.DDL
}

// New returns a Migrator for db. It panics if db's dialect doesn't implement
// dialect.DDL.
func New(db *pack.DB) *Migrator {
	ddl, ok := db.Dialect().(dialect.DDL)
	if !ok {
		panic(fmt.Sprintf("pack/migrate: dialect %q does not implement dialect.DDL", db.Dialect().Name()))
	}
	return &Migrator{db: db, ddl: ddl}
}

func sqlTable(table *schema.Table) sqlbuild.Table {
	return sqlbuild.Table{Name: table.Name}
}

// CreateTable creates one table per model, deriving the full column set
// (types, primary key, unique, not-null, default) from the same struct tags
// Create[T]/Query[T] already use.
func (m *Migrator) CreateTable(ctx context.Context, dst ...any) error {
	for _, d := range dst {
		table := schemaForOrPanic(d)

		b := sqlbuild.CreateTable(sqlTable(table))
		for i := range table.Fields {
			f := &table.Fields[i]
			b = b.Column(f.Column, f.Type, toColumnSpec(f.Options))
		}

		sqlText, args, err := b.Render(m.db.Dialect())
		if err != nil {
			return err
		}
		if _, err := m.db.ExecContext(ctx, "CreateTable", table.GoType.Name(), sqlText, args); err != nil {
			return err
		}
	}

	return nil
}

func (m *Migrator) DropTable(ctx context.Context, dst any, opts ...DropOption) error {
	table := schemaForOrPanic(dst)
	cfg := applyDropOptions(opts)

	if !cfg.withoutIfExists {
		exists, err := m.HasTable(ctx, dst)
		if err != nil {
			return err
		}
		if !exists {
			return nil
		}
	}

	sqlText, args, err := sqlbuild.DropTable(sqlTable(table)).Render(m.db.Dialect())
	if err != nil {
		return err
	}
	_, err = m.db.ExecContext(ctx, "DropTable", table.GoType.Name(), sqlText, args)
	return err
}

// HasTable reports whether dst's table exists.
func (m *Migrator) HasTable(ctx context.Context, dst any) (bool, error) {
	table := schemaForOrPanic(dst)
	sqlText, args := m.ddl.HasTableSQL(table.Name)
	return m.queryExists(ctx, "HasTable", table.GoType.Name(), sqlText, args)
}

// RenameTable renames dst's table to newName.
func (m *Migrator) RenameTable(ctx context.Context, dst any, newName string) error {
	table := schemaForOrPanic(dst)

	sqlText, args, err := sqlbuild.RenameTable(sqlTable(table), newName).Render(m.db.Dialect())
	if err != nil {
		return err
	}
	_, err = m.db.ExecContext(ctx, "RenameTable", table.GoType.Name(), sqlText, args)
	return err
}

func (m *Migrator) queryExists(ctx context.Context, op, model, sqlText string, args []any) (bool, error) {
	rows, err := m.db.QueryContext(ctx, op, model, sqlText, args)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return false, err
		}
		return false, nil
	}

	var exists bool
	if err := rows.Scan(&exists); err != nil {
		return false, err
	}

	return exists, rows.Err()
}
