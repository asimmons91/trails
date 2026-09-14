package scan

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/asimmons91/trails/pack/internal/schema"
)

type Plan struct {
	fields []*schema.Field
}

func NewPlan(table *schema.Table, columns []string) *Plan {
	fields := make([]*schema.Field, len(columns))
	for i, name := range columns {
		fields[i] = table.FieldsByColumn[name]
	}

	return &Plan{fields: fields}
}

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

type jsonDest struct {
	target reflect.Value
}

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
