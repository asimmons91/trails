package pack

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/asimmons91/trails/pack/internal/scan"
	"github.com/asimmons91/trails/pack/internal/schema"
)

// scanAllRows scans every row of rows into a T via internal/scan, firing
// each result's AfterScan hook (if T implements one) before returning.
func scanAllRows[T any](ctx context.Context, rows *sql.Rows, table *schema.Table) ([]T, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	plan := scan.NewPlan(table, cols)
	result, err := scan.All[T](rows, plan)
	if err != nil {
		return nil, err
	}

	if table.Hooks.AfterScan {
		for i := range result {
			if as, ok := any(&result[i]).(AfterScanner); ok {
				if err := as.AfterScan(ctx); err != nil {
					return nil, fmt.Errorf("pack: AfterScan hook on %s: %w", table.GoType.Name(), err)
				}
			}
		}
	}

	return result, nil
}

// scanOneRow scans the single next row of rows into a T, building a fresh
// scan.Plan first (see scanOneWithPlan for reusing an existing one).
func scanOneRow[T any](ctx context.Context, rows *sql.Rows, table *schema.Table) (T, error) {
	var zero T
	cols, err := rows.Columns()
	if err != nil {
		return zero, err
	}

	return scanOneWithPlan[T](ctx, rows, scan.NewPlan(table, cols), table)
}

// scanOneWithPlan scans the single next row of rows into a T using an
// already-built plan (so Query[T].Rows doesn't rebuild one per row),
// firing its AfterScan hook if T implements one.
func scanOneWithPlan[T any](ctx context.Context, rows *sql.Rows, plan *scan.Plan, table *schema.Table) (T, error) {
	var zero T
	row, err := scan.One[T](rows, plan)
	if err != nil {
		return zero, err
	}

	if table.Hooks.AfterScan {
		if as, ok := any(&row).(AfterScanner); ok {
			if err := as.AfterScan(ctx); err != nil {
				return zero, fmt.Errorf("pack: AfterScan hook on %s: %w", table.GoType.Name(), err)
			}
		}
	}

	return row, nil
}
