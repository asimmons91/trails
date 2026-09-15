package migrate

import (
	"fmt"
	"reflect"

	"github.com/asimmons91/trails/pack/dialect"
	"github.com/asimmons91/trails/pack/internal/schema"
)

func schemaForOrPanic(dst any) *schema.Table {
	t := reflect.TypeOf(dst)
	table, err := schema.For(t)
	if err != nil {
		panic(fmt.Sprintf("pack/migrate: %s: %v", t, err))
	}
	return table
}

func fieldByGoName(table *schema.Table, goName string) (*schema.Field, bool) {
	for i := range table.Fields {
		if table.Fields[i].GoName == goName {
			return &table.Fields[i], true
		}
	}

	return nil, false
}

func fieldByGoNameOrPanic(table *schema.Table, goName string) *schema.Field {
	f, ok := fieldByGoName(table, goName)
	if !ok {
		panic(fmt.Sprintf("pack/migrate: %s has no field named %q", table.GoType.Name(), goName))
	}

	return f
}

func resolveColumnName(table *schema.Table, field string) string {
	if f, ok := fieldByGoName(table, field); ok {
		return f.Column
	}

	return field
}

func relationByGoName(table *schema.Table, goName string) (*schema.Relation, bool) {
	for i := range table.Relations {
		if table.Relations[i].GoName == goName {
			return &table.Relations[i], true
		}
	}

	return nil, false
}

func toColumnSpec(opts schema.ColumnOptions) dialect.ColumnSpec {
	return dialect.ColumnSpec{
		PrimaryKey:    opts.PK,
		AutoIncrement: opts.AutoIncrement,
		NotNull:       opts.NotNull,
		Unique:        opts.Unique,
		HasDefault:    opts.HasDefault,
		Default:       opts.Default,
		SQLType:       opts.SQLType,
	}
}
