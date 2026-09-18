// Package scan builds a reusable column-to-field scan Plan from a
// schema.Table and a result set's column list, then scans rows into T via
// One or All — the low-level row-to-struct half of every pack read path.
package scan

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/asimmons91/trails/pack/internal/schema"
)

// Plan maps a result set's columns, in order, to the schema.Field each one
// scans into. Build one with NewPlan and reuse it across every row of the
// same query.
type Plan struct {
	fields []*schema.Field
}

// NewPlan builds a Plan mapping each of columns to its schema.Field in
// table, in the same order. A column table doesn't recognize is mapped to
// nil and scanned into a discard target instead of erroring, so a
// SELECT *-style query with extra columns doesn't break scanning.
func NewPlan(table *schema.Table, columns []string) *Plan {
	fields := make([]*schema.Field, len(columns))
	for i, name := range columns {
		fields[i] = table.FieldsByColumn[name]
	}

	return &Plan{fields: fields}
}

// dests returns the scan destinations for one row of v, in p's column
// order. A json/jsonb-tagged field is routed through jsonDest so its raw
// column bytes are unmarshaled into the Go type instead of assigned
// directly.
func (p *Plan) dests(v reflect.Value) []any {
	dests := make([]any, len(p.fields))
	for i, f := range p.fields {
		if f == nil {
			var discard any
			dests[i] = &discard
			continue
		}

		fv := v.FieldByIndex(f.Index)
		if f.Options.SQLType == "json" || f.Options.SQLType == "jsonb" {
			dests[i] = &jsonDest{target: fv}
			continue
		}

		dests[i] = fv.Addr().Interface()
	}

	return dests
}

// jsonDest implements sql.Scanner so a json/jsonb-tagged field can be
// scanned straight from its raw column value into its Go type via
// json.Unmarshal, instead of database/sql trying (and failing) to assign
// the raw bytes/string directly.
type jsonDest struct {
	target reflect.Value
}

// Scan unmarshals src (expected to be []byte, string, or nil) as JSON into
// d's target.
func (d *jsonDest) Scan(src any) error {
	if src == nil {
		d.target.SetZero()
		return nil
	}

	var b []byte
	switch v := src.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return fmt.Errorf("pack: json column: unsupported source type %T", src)
	}

	return json.Unmarshal(b, d.target.Addr().Interface())
}

// One scans the next row of rows into a T using p, returning sql.ErrNoRows
// if there isn't one.
func One[T any](rows *sql.Rows, p *Plan) (T, error) {
	var zero T
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return zero, err
		}

		return zero, sql.ErrNoRows
	}

	var out T
	v := reflect.ValueOf(&out).Elem()
	if err := rows.Scan(p.dests(v)...); err != nil {
		return zero, err
	}

	return out, nil
}

// All scans every remaining row of rows into a []T using p.
func All[T any](rows *sql.Rows, p *Plan) ([]T, error) {
	var out []T
	for rows.Next() {
		var row T
		v := reflect.ValueOf(&row).Elem()
		if err := rows.Scan(p.dests(v)...); err != nil {
			return nil, err
		}
		out = append(out, row)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}
