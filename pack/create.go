package pack

import (
	"context"
	"fmt"
	"reflect"

	"github.com/asimmons91/trails/pack/internal/schema"
	"github.com/asimmons91/trails/pack/internal/sqlbuild"
)

// Create inserts row, firing a BeforeInsert hook first (if T implements
// one) and an AfterInsert hook last (if implemented). Any auto-increment
// or defaulted column left at its zero value is backfilled onto row after
// the insert, via RETURNING where the dialect supports it (see
// createWithoutReturning otherwise). Returns ErrZeroCompositeKey if T has
// a composite primary key still at its zero value — composite keys are
// never server-generated.
func Create[T any](ctx context.Context, db *DB, row *T, opts ...WriteOption) error {
	t := reflect.TypeFor[T]()
	table, err := schema.For(t)
	if err != nil {
		panic(fmt.Sprintf("pack: Create[%s]: %v", t.Name(), err))
	}

	cfg := &writeConfig{}
	for _, o := range opts {
		o.applyToWriteConfig(cfg)
	}

	if table.Hooks.BeforeInsert {
		if bi, ok := any(row).(BeforeInserter); ok {
			if err := bi.BeforeInsert(ctx); err != nil {
				return fmt.Errorf("pack: BeforeInsert hook on %s: %w", table.GoType.Name(), err)
			}
		}
	}

	rv := reflect.ValueOf(row).Elem()
	if table.PKIsComposite && compositeKeyIsZero(table, rv) {
		return &ErrZeroCompositeKey{Model: table.GoType.Name()}
	}

	var assignments []sqlbuild.Assignment
	var returning []schema.Field
	for i := range table.Fields {
		f := &table.Fields[i]
		fv := rv.FieldByIndex(f.Index)

		if f.Options.AutoIncrement || (f.Options.HasDefault && fv.IsZero()) {
			returning = append(returning, *f)
			continue
		}

		val, err := bindableValue(table.GoType.Name(), f, fv)
		if err != nil {
			return err
		}
		assignments = append(assignments, sqlbuild.Set(sqlbuild.Col(f.Column), val))
	}

	ib := sqlbuild.Insert(sqlbuild.Table{Name: table.Name, Alias: table.Alias}).Values(assignments...)
	if cfg.conflict != nil {
		if cfg.conflict.doNothing {
			ib = ib.OnConflictDoNothing(cfg.conflict.cols...)
			returning = nil
		} else {
			ib = ib.OnConflictDoUpdate(cfg.conflict.cols, cfg.conflict.doUpdate...)
		}
	}

	if len(returning) == 0 {
		sqlText, args, err := ib.Render(db.dialect)
		if err != nil {
			return err
		}
		if _, err := db.execContext(ctx, "Create", table.GoType.Name(), sqlText, args); err != nil {
			return err
		}
		return fireAfterInsert(ctx, table, row)
	}

	if !db.dialect.SupportsReturning() {
		return createWithoutReturning(ctx, db, table, ib, rv, row, returning)
	}

	retCols := make([]sqlbuild.Column, len(returning))
	for i, f := range returning {
		retCols[i] = sqlbuild.Col(f.Column)
	}
	sqlText, args, err := ib.Returning(retCols...).Render(db.dialect)
	if err != nil {
		return err
	}

	rows, err := db.queryContext(ctx, "Create", table.GoType.Name(), sqlText, args)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		return fmt.Errorf("pack: Create(%s): INSERT ... RETURNING produced no row", table.GoType.Name())
	}

	dests := make([]any, len(returning))
	for i, f := range returning {
		dests[i] = returningDest(row, rv, f)
	}
	if err := rows.Scan(dests...); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}

	return fireAfterInsert(ctx, table, row)
}

// createWithoutReturning is Create's fallback for a dialect without
// RETURNING support (see dialect.Dialect.SupportsReturning): it inserts
// row, pulls an auto-increment id via sql.Result.LastInsertId, and — if
// any other defaulted column still needs a value — runs a follow-up
// SELECT keyed by the primary key to fetch the rest.
func createWithoutReturning(ctx context.Context, db *DB, table *schema.Table, ib *sqlbuild.InsertBuilder, rv reflect.Value, row any, returning []schema.Field) error {
	sqlText, args, err := ib.Render(db.dialect)
	if err != nil {
		return err
	}

	res, err := db.execContext(ctx, "Create", table.GoType.Name(), sqlText, args)
	if err != nil {
		return err
	}

	var remaining []schema.Field
	for _, f := range returning {
		if !f.Options.AutoIncrement {
			remaining = append(remaining, f)
			continue
		}

		id, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("pack: Create(%s): LastInsertId: %w", table.GoType.Name(), err)
		}
		if err := assignInt64(returningDest(row, rv, f), id); err != nil {
			return fmt.Errorf("pack: Create(%s): %w", table.GoType.Name(), err)
		}
	}

	if len(remaining) == 0 {
		return fireAfterInsert(ctx, table, row)
	}

	retCols := make([]sqlbuild.Column, len(remaining))
	for i, f := range remaining {
		retCols[i] = sqlbuild.Col(f.Column)
	}

	sqlText, args, err = sqlbuild.Select(sqlbuild.Table{Name: table.Name}).
		Columns(retCols...).
		Where(pkPredicateFromRow(table, rv)).
		Render(db.dialect)
	if err != nil {
		return err
	}

	rows, err := db.queryContext(ctx, "Create", table.GoType.Name(), sqlText, args)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		return fmt.Errorf("pack: Create(%s): follow-up SELECT for default columns produced no row", table.GoType.Name())
	}

	dests := make([]any, len(remaining))
	for i, f := range remaining {
		dests[i] = returningDest(row, rv, f)
	}
	if err := rows.Scan(dests...); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}

	return fireAfterInsert(ctx, table, row)
}

// fireAfterInsert calls row's AfterInsert hook if table's model implements
// one.
func fireAfterInsert(ctx context.Context, table *schema.Table, row any) error {
	if !table.Hooks.AfterInsert {
		return nil
	}

	ai, ok := row.(AfterInserter)
	if !ok {
		return nil
	}

	if err := ai.AfterInsert(ctx); err != nil {
		return fmt.Errorf("pack: AfterInsert hook on %s: %w", table.GoType.Name(), err)

	}

	return nil
}

// returningDest picks the address to scan a generated value for field f
// into: f's own struct field, except for the primary key, which — if row
// implements pkAddressable (i.e. embeds Model[ID]) — is written through
// that interface instead.
func returningDest(row any, rv reflect.Value, f schema.Field) any {
	if f.Options.PK {
		if pa, ok := row.(pkAddressable); ok {
			return pa.pkFieldAddr()
		}
	}

	return rv.FieldByIndex(f.Index).Addr().Interface()
}
