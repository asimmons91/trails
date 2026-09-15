package migrate

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/asimmons91/trails/pack"
	"github.com/asimmons91/trails/pack/dialect"
	"github.com/asimmons91/trails/pack/internal/schema"
	"github.com/asimmons91/trails/pack/internal/sqlbuild"
)

func (m *Migrator) AlterColumn(ctx context.Context, dst any, field string) error {
	table := schemaForOrPanic(dst)
	f := fieldByGoNameOrPanic(table, field)

	sqlText, err := m.ddl.AlterColumnSQL(table.Name, f.Column, f.Type, toColumnSpec(f.Options))
	switch {
	case errors.Is(err, dialect.ErrRequiresTableRebuild):
		return m.alterColumnViaRebuild(ctx, table)
	case err != nil:
		return err
	}

	_, err = m.db.ExecContext(ctx, "AlterColumn", table.GoType.Name(), sqlText, nil)
	return err
}

func (m *Migrator) alterColumnViaRebuild(ctx context.Context, table *schema.Table) error {
	if m.db.InTransaction() {
		return ErrAlterColumnRequiresTopLevelConnection
	}

	return m.db.PinnedConn(ctx, func(pinned *pack.DB) error {
		if _, err := pinned.ExecContext(ctx, "PRAGMA", "", "PRAGMA foreign_keys = OFF", nil); err != nil {
			return err
		}

		if err := pinned.Tx(ctx, func(tx *pack.DB) error {
			return rebuildTable(ctx, tx, table)
		}); err != nil {
			return err
		}

		rows, err := pinned.QueryContext(ctx, "PRAGMA", "", "PRAGMA foreign_key_check", nil)
		if err != nil {
			return err
		}
		violations, err := scanForeignKeyCheckRows(rows)
		if err != nil {
			return err
		}
		if len(violations) > 0 {
			return &ErrForeignKeyCheckFailed{Table: table.Name, Rows: violations}
		}

		_, err = pinned.ExecContext(ctx, "PRAGMA", "", "PRAGMA foreign_keys = ON", nil)
		return err
	})
}

func rebuildTable(ctx context.Context, tx *pack.DB, table *schema.Table) error {
	indexSQL, err := captureIndexSQL(ctx, tx, table.Name)
	if err != nil {
		return err
	}

	tmpName := table.Name + "__pack_rebuild"

	b := sqlbuild.CreateTable(sqlbuild.Table{Name: tmpName})
	colNames := make([]string, 0, len(table.Fields))
	for i := range table.Fields {
		f := &table.Fields[i]
		b = b.Column(f.Column, f.Type, toColumnSpec(f.Options))
		colNames = append(colNames, f.Column)
	}

	createSQL, _, err := b.Render(tx.Dialect())
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "CreateTable", table.GoType.Name(), createSQL, nil); err != nil {
		return err
	}

	colList := quoteJoin(tx.Dialect(), colNames)
	insertSQL := "INSERT INTO " + tx.Dialect().QuoteIdent(tmpName) + " (" + colList + ") SELECT " + colList +
		" FROM " + tx.Dialect().QuoteIdent(table.Name)
	if _, err := tx.ExecContext(ctx, "Raw", table.GoType.Name(), insertSQL, nil); err != nil {
		return err
	}

	dropSQL, _, err := sqlbuild.DropTable(sqlbuild.Table{Name: table.Name}).Render(tx.Dialect())
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DropTable", table.GoType.Name(), dropSQL, nil); err != nil {
		return err
	}

	renameSQL, _, err := sqlbuild.RenameTable(sqlbuild.Table{Name: tmpName}, table.Name).Render(tx.Dialect())
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "RenameTable", table.GoType.Name(), renameSQL, nil); err != nil {
		return err
	}

	for _, idxSQL := range indexSQL {
		if _, err := tx.ExecContext(ctx, "Raw", table.GoType.Name(), idxSQL, nil); err != nil {
			return err
		}
	}

	return nil
}

func captureIndexSQL(ctx context.Context, tx *pack.DB, table string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, "Raw", "",
		"SELECT sql FROM sqlite_master WHERE type = 'index' AND tbl_name = ?1 AND sql IS NOT NULL",
		[]any{table})
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}

	return out, rows.Err()
}

func scanForeignKeyCheckRows(rows *sql.Rows) ([]map[string]any, error) {
	defer func() { _ = rows.Close() }()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var out []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}

		row := make(map[string]any, len(cols))
		for i, c := range cols {
			row[c] = vals[i]
		}
		out = append(out, row)
	}

	return out, rows.Err()
}

func quoteJoin(d dialect.Dialect, names []string) string {
	parts := make([]string, len(names))
	for i, n := range names {
		parts[i] = d.QuoteIdent(n)
	}
	return strings.Join(parts, ", ")
}
