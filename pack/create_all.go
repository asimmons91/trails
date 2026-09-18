package pack

import (
	"context"
	"fmt"
	"reflect"

	"github.com/asimmons91/trails/pack/internal/schema"
	"github.com/asimmons91/trails/pack/internal/sqlbuild"
)

// defaultCreateAllBatchSize is how many rows CreateAll inserts per
// statement unless overridden by WithBatchSize.
const defaultCreateAllBatchSize = 1000

// maxPostgresParams caps CreateAll's batch size so a single multi-row
// INSERT never exceeds Postgres's bound-parameter limit, regardless of
// WithBatchSize.
const maxPostgresParams = 65535

// CreateAll inserts rows in batches (WithBatchSize, default
// defaultCreateAllBatchSize rows, further capped so Postgres's parameter
// limit is never exceeded), one multi-row INSERT per batch, firing
// before/after-insert hooks per row if T implements them. Like Create, it
// backfills auto-increment/defaulted columns onto each row afterward —
// via a single RETURNING clause per batch where the dialect supports it,
// or createAllWithoutReturning otherwise. Returns ErrZeroCompositeKey if
// any row has a composite primary key still at its zero value.
func CreateAll[T any](ctx context.Context, db *DB, rows []*T, opts ...WriteOption) error {
	if len(rows) == 0 {
		return nil
	}

	t := reflect.TypeFor[T]()
	table, err := schema.For(t)
	if err != nil {
		panic(fmt.Sprintf("pack: CreateAll[%s]: %v", t.Name(), err))
	}

	cfg := &writeConfig{batchSize: defaultCreateAllBatchSize}
	for _, o := range opts {
		o.applyToWriteConfig(cfg)
	}
	batchSize := cfg.batchSize
	if batchSize <= 0 {
		batchSize = defaultCreateAllBatchSize
	}

	if table.Hooks.BeforeInsert {
		for _, row := range rows {
			if bi, ok := any(row).(BeforeInserter); ok {
				if err := bi.BeforeInsert(ctx); err != nil {
					return fmt.Errorf("pack: BeforeInsert hook on %s: %w", table.GoType.Name(), err)
				}
			}
		}
	}

	var insertFields, returningFields []schema.Field
	for _, f := range table.Fields {
		if f.Options.AutoIncrement {
			returningFields = append(returningFields, f)
		} else {
			insertFields = append(insertFields, f)
		}
	}

	doNothing := cfg.conflict != nil && cfg.conflict.doNothing
	if doNothing {
		returningFields = nil
	}

	allAssignments := make([][]sqlbuild.Assignment, len(rows))
	for ri, row := range rows {
		rv := reflect.ValueOf(row).Elem()
		if table.PKIsComposite && compositeKeyIsZero(table, rv) {
			return &ErrZeroCompositeKey{Model: table.GoType.Name()}
		}

		rowAssignments := make([]sqlbuild.Assignment, len(insertFields))
		for i, f := range insertFields {
			val, err := bindableValue(table.GoType.Name(), &f, rv.FieldByIndex(f.Index))
			if err != nil {
				return err
			}
			rowAssignments[i] = sqlbuild.Set(sqlbuild.Col(f.Column), val)
		}
		allAssignments[ri] = rowAssignments
	}

	if len(insertFields) > 0 {
		if maxByParams := maxPostgresParams / len(insertFields); maxByParams < batchSize {
			batchSize = maxByParams
		}
	}

	if batchSize < 1 {
		batchSize = 1
	}

	retCols := make([]sqlbuild.Column, len(returningFields))
	for i, f := range returningFields {
		retCols[i] = sqlbuild.Col(f.Column)
	}

	for start := 0; start < len(rows); start += batchSize {
		end := start + batchSize
		if end > len(rows) {
			end = len(rows)
		}
		chunkRows := rows[start:end]
		chunkAssignments := allAssignments[start:end]

		ib := sqlbuild.Insert(sqlbuild.Table{Name: table.Name})
		for _, a := range chunkAssignments {
			ib = ib.Values(a...)
		}
		if cfg.conflict != nil {
			if cfg.conflict.doNothing {
				ib = ib.OnConflictDoNothing(cfg.conflict.cols...)
			} else {
				ib = ib.OnConflictDoUpdate(cfg.conflict.cols, cfg.conflict.doUpdate...)
			}
		}

		switch {
		case len(returningFields) == 0:
			sqlText, args, err := ib.Render(db.dialect)
			if err != nil {
				return err
			}
			if _, err := db.execContext(ctx, "CreateAll", table.GoType.Name(), sqlText, args); err != nil {
				return err
			}
		case db.dialect.SupportsReturning():
			sqlText, args, err := ib.Returning(retCols...).Render(db.dialect)
			if err != nil {
				return err
			}
			if err := scanCreateAllReturning[T](ctx, db, table, sqlText, args, chunkRows, returningFields); err != nil {
				return err
			}
		default:
			if err := createAllWithoutReturning(ctx, db, table, ib, chunkRows, returningFields); err != nil {
				return err
			}
		}

		if table.Hooks.AfterInsert {
			for _, row := range chunkRows {
				if ai, ok := any(row).(AfterInserter); ok {
					if err := ai.AfterInsert(ctx); err != nil {
						return fmt.Errorf("pack: AfterInsert hook on %s: %w", table.GoType.Name(), err)
					}
				}
			}
		}
	}

	return nil
}

// createAllWithoutReturning is CreateAll's fallback for a dialect without
// RETURNING support: it inserts the batch, then backfills each row's
// auto-increment field from a single LastInsertId, relying on the
// database allocating a contiguous block of ids for a multi-row INSERT
// (true for MySQL and SQLite) so row i's id is firstID+i. It does not
// handle non-auto-increment defaulted columns — those dialects are
// expected to support RETURNING instead.
func createAllWithoutReturning[T any](ctx context.Context, db *DB, table *schema.Table, ib *sqlbuild.InsertBuilder, chunkRows []*T, returningFields []schema.Field) error {
	sqlText, args, err := ib.Render(db.dialect)
	if err != nil {
		return err
	}

	res, err := db.execContext(ctx, "CreateAll", table.GoType.Name(), sqlText, args)
	if err != nil {
		return err
	}

	firstID, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("pack: CreateAll(%s): LastInsertId: %w", table.GoType.Name(), err)
	}

	for i, row := range chunkRows {
		rv := reflect.ValueOf(row).Elem()
		for _, f := range returningFields {
			if err := assignInt64(returningDest(row, rv, f), firstID+int64(i)); err != nil {
				return fmt.Errorf("pack: CreateAll(%s): %w", table.GoType.Name(), err)
			}
		}
	}

	return nil
}

// scanCreateAllReturning scans one RETURNING row per chunkRows entry, in
// the same order the batch was inserted, into each row's returningFields.
func scanCreateAllReturning[T any](ctx context.Context, db *DB, table *schema.Table, sqlText string, args []any, chunkRows []*T, returningFields []schema.Field) error {
	rows, err := db.queryContext(ctx, "CreateAll", table.GoType.Name(), sqlText, args)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	for _, row := range chunkRows {
		rv := reflect.ValueOf(row).Elem()
		if !rows.Next() {
			if err := rows.Err(); err != nil {
				return err
			}
			return fmt.Errorf("pack: CreateAll(%s): INSERT ... RETURNING produced fewer rows than expected", table.GoType.Name())
		}
		dests := make([]any, len(returningFields))
		for i, f := range returningFields {
			dests[i] = returningDest(row, rv, f)
		}
		if err := rows.Scan(dests...); err != nil {
			return err
		}
	}

	return rows.Err()
}
