package migrate

import (
	"context"
	"fmt"

	"github.com/asimmons91/trails/pack/internal/schema"
	"github.com/asimmons91/trails/pack/internal/sqlbuild"
)

// CreateConstraint creates a foreign-key constraint on dst's table for its
// belongs_to relation named relationName (only belongs_to is supported —
// see ErrConstraintWrongRelationKind — since that's the side the FK column
// lives on). The referenced column defaults to the target's single-column
// primary key, and the constraint name defaults to "fk_<table>_<fk
// column>"; both can be overridden via opts.
func (m *Migrator) CreateConstraint(ctx context.Context, dst any, relationName string, opts ...ConstraintOption) error {
	table := schemaForOrPanic(dst)

	rel, ok := relationByGoName(table, relationName)
	if !ok {
		return &ErrRelationNotFound{Model: table.GoType.Name(), Relation: relationName}
	}
	if rel.Kind != schema.BelongsTo {
		return &ErrConstraintWrongRelationKind{Model: table.GoType.Name(), Relation: relationName, Kind: rel.Kind.String()}
	}

	targetTable, err := schema.For(rel.TargetType)
	if err != nil {
		panic(fmt.Sprintf("pack/migrate: CreateConstraint(%s, %q): %v", table.GoType.Name(), relationName, err))
	}

	ref := rel.Ref
	if ref == "" {
		if len(targetTable.PK) != 1 {
			return fmt.Errorf(
				"pack/migrate: CreateConstraint(%s, %q): %s has no single-column primary key to default ref: to; specify ref: explicitly on the relation tag",
				table.GoType.Name(), relationName, targetTable.GoType.Name(),
			)
		}
		ref = targetTable.PK[0].Column
	}

	cfg := applyConstraintOptions(opts)
	name := cfg.name
	if name == "" {
		name = "fk_" + table.Name + "_" + rel.FK
	}

	sqlText, args, err := sqlbuild.CreateForeignKeyConstraint(
		sqlTable(table), name, []string{rel.FK}, targetTable.Name, []string{ref},
	).Render(m.db.Dialect())
	if err != nil {
		return err
	}
	_, err = m.db.ExecContext(ctx, "CreateConstraint", table.GoType.Name(), sqlText, args)
	return err
}

// DropConstraint drops the named constraint from dst's table. It's a no-op
// if the constraint doesn't already exist, unless opts includes
// WithoutIfExists.
func (m *Migrator) DropConstraint(ctx context.Context, dst any, name string, opts ...DropOption) error {
	table := schemaForOrPanic(dst)
	cfg := applyDropOptions(opts)

	if !cfg.withoutIfExists {
		exists, err := m.HasConstraint(ctx, dst, name)
		if err != nil {
			return err
		}
		if !exists {
			return nil
		}
	}

	sqlText, args, err := sqlbuild.DropConstraint(sqlTable(table), name).Render(m.db.Dialect())
	if err != nil {
		return err
	}
	_, err = m.db.ExecContext(ctx, "DropConstraint", table.GoType.Name(), sqlText, args)
	return err
}

// HasConstraint reports whether the named constraint exists on dst's table.
func (m *Migrator) HasConstraint(ctx context.Context, dst any, name string) (bool, error) {
	table := schemaForOrPanic(dst)
	sqlText, args := m.ddl.HasConstraintSQL(table.Name, name)
	return m.queryExists(ctx, "HasConstraint", table.GoType.Name(), sqlText, args)
}
