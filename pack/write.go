package pack

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"

	"github.com/asimmons91/trails/pack/internal/schema"
	"github.com/asimmons91/trails/pack/internal/sqlbuild"
)

func bindableValue(modelName string, f *schema.Field, fv reflect.Value) (any, error) {
	if f.Options.NullZero && fv.IsZero() {
		return nil, nil
	}

	if f.Options.SQLType == "json" || f.Options.SQLType == "jsonb" {
		v := fv.Interface()
		if fv.Kind() == reflect.Slice && fv.IsNil() {
			v = reflect.MakeSlice(fv.Type(), 0, 0).Interface()
		}
		b, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("pack: %s.%s: marshal jsonb: %w", modelName, f.GoName, err)
		}

		return b, nil
	}

	switch fv.Kind() {
	case reflect.Uint, reflect.Uint64:
		if u := fv.Uint(); u > math.MaxInt64 {
			return nil, &ErrUintOverflow{Model: modelName, Field: f.GoName, Column: f.Column, Value: u}
		}
	}

	return fv.Interface(), nil
}

func writeAllAssignments(modelName string, table *schema.Table, rv reflect.Value) ([]sqlbuild.Assignment, error) {
	out := make([]sqlbuild.Assignment, 0, len(table.Fields))
	for i := range table.Fields {
		f := &table.Fields[i]
		if f.Options.PK {
			continue
		}

		val, err := bindableValue(modelName, f, rv.FieldByIndex(f.Index))
		if err != nil {
			return nil, err
		}
		out = append(out, sqlbuild.Set(sqlbuild.Col(f.Column), val))
	}

	return out, nil
}

func compositeKeyIsZero(table *schema.Table, rv reflect.Value) bool {
	for _, f := range table.PK {
		if !rv.FieldByIndex(f.Index).IsZero() {
			return false
		}
	}

	return true
}

func pkPredicate(table *schema.Table, id any, alias string) (sqlbuild.Predicate, error) {
	if len(table.PK) == 0 {
		return sqlbuild.Predicate{}, fmt.Errorf(
			"pack: %s has no primary-key column mapped (embed Model[ID] or tag a field pk)",
			table.GoType.Name(),
		)
	}

	col := func(name string) sqlbuild.Column {
		if alias == "" {
			return sqlbuild.Col(name)
		}
		return sqlbuild.QualifiedCol(alias, name)
	}

	if !table.PKIsComposite {
		return sqlbuild.Eq(col(table.PK[0].Column), id), nil
	}

	v := reflect.ValueOf(id)
	if v.Kind() != reflect.Struct || v.NumField() != len(table.PK) {
		return sqlbuild.Predicate{}, fmt.Errorf(
			"pack: %s composite key value has the wrong shape for its %d-field key",
			table.GoType.Name(), len(table.PK),
		)
	}

	preds := make([]sqlbuild.Predicate, len(table.PK))
	for i, f := range table.PK {
		preds[i] = sqlbuild.Eq(col(f.Column), v.Field(i).Interface())
	}

	return sqlbuild.And(preds...), nil
}

func pkPredicateFromRow(table *schema.Table, rv reflect.Value) sqlbuild.Predicate {
	preds := make([]sqlbuild.Predicate, len(table.PK))
	for i, f := range table.PK {
		preds[i] = sqlbuild.Eq(sqlbuild.Col(f.Column), rv.FieldByIndex(f.Index).Interface())
	}

	if len(preds) == 1 {
		return preds[0]
	}

	return sqlbuild.And(preds...)
}

func assignInt64(dest any, v int64) error {
	rv := reflect.ValueOf(dest)
	if rv.Kind() != reflect.Pointer {
		return fmt.Errorf("pack: internal: LastInsertId destination %T is not a pointer", dest)
	}

	elem := rv.Elem()
	switch elem.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		elem.SetInt(v)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		elem.SetUint(uint64(v))
	default:
		return fmt.Errorf("pack: internal: LastInsertId destination has unsupported kind %s", elem.Kind())
	}

	return nil
}
