package migrate

import (
	"fmt"
	"reflect"

	"github.com/asimmons91/trails/pack/dialect"
	"github.com/asimmons91/trails/pack/internal/schema"
)

// schemaForOrPanic reflects dst into its schema.Table, panicking if dst
// isn't a valid pack model.
func schemaForOrPanic(dst any) *schema.Table {
	t := reflect.TypeOf(dst)
	table, err := schema.For(t)
	if err != nil {
		panic(fmt.Sprintf("pack/migrate: %s: %v", t, err))
	}
	return table
}

// fieldByGoName looks up table's field by its Go struct field name.
func fieldByGoName(table *schema.Table, goName string) (*schema.Field, bool) {
	for i := range table.Fields {
		if table.Fields[i].GoName == goName {
			return &table.Fields[i], true
		}
	}

	return nil, false
}

// fieldByGoNameOrPanic is fieldByGoName, panicking instead of returning ok=false.
func fieldByGoNameOrPanic(table *schema.Table, goName string) *schema.Field {
	f, ok := fieldByGoName(table, goName)
	if !ok {
		panic(fmt.Sprintf("pack/migrate: %s has no field named %q", table.GoType.Name(), goName))
	}

	return f
}

// resolveColumnName returns field's DB column name if it names a Go field
// on table, otherwise field itself (already assumed to be a column name).
func resolveColumnName(table *schema.Table, field string) string {
	if f, ok := fieldByGoName(table, field); ok {
		return f.Column
	}

	return field
}

// relationByGoName looks up table's relation by its Go struct field name.
func relationByGoName(table *schema.Table, goName string) (*schema.Relation, bool) {
	for i := range table.Relations {
		if table.Relations[i].GoName == goName {
			return &table.Relations[i], true
		}
	}

	return nil, false
}

// toColumnSpec converts a schema.ColumnOptions (parsed from a model's
// struct tags) into the dialect.ColumnSpec shape dialect.DDL's
// column-generating methods expect.
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
