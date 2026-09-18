package pack

import (
	"context"
	"fmt"
	"reflect"

	"github.com/asimmons91/trails/pack/internal/schema"
)

// Raw runs sqlText verbatim (with args bound to its placeholders) and
// scans the results into []T the same way Find does, for queries
// Query[T] can't express. T must still be a valid pack model — its
// mapped columns are matched against the result set by name.
func Raw[T any](ctx context.Context, db *DB, sqlText string, args ...any) ([]T, error) {
	t := reflect.TypeFor[T]()
	table, err := schema.For(t)
	if err != nil {
		panic(fmt.Sprintf("pack: Raw[%s]: %v", t.Name(), err))
	}

	rows, err := db.queryContext(ctx, "Raw", t.Name(), sqlText, args)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return scanAllRows[T](ctx, rows, table)
}
