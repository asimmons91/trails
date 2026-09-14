package schema

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"reflect"
	"sync"
	"time"
)

type Field struct {
	GoName  string
	Column  string
	Index   []int // for reflect.Value.FieldByIndex. Prevents looking up by name in hot path
	Type    reflect.Type
	Options ColumnOptions
}

type Relation struct {
	GoName     string
	Index      []int
	Kind       RelationKind
	TargetType reflect.Type
	Slice      bool
	FK         string
	Ref        string
}

type Table struct {
	GoType         reflect.Type
	Name           string
	Alias          string
	Fields         []Field
	FieldsByColumn map[string]*Field
	PK             []Field
	PKIsComposite  bool
	Relations      []Relation
	Hooks          Hooks
}

type Hooks struct {
	BeforeInsert bool
	AfterInsert  bool
	BeforeUpdate bool
	AfterUpdate  bool
	BeforeDelete bool
	AfterDelete  bool
	AfterScan    bool
}

const packStructTag = "db"

var (
	tableNamerType = reflect.TypeFor[interface{ TableName() string }]()
	valuerType     = reflect.TypeFor[driver.Valuer]()
	scannerType    = reflect.TypeFor[sql.Scanner]()
	timeType       = reflect.TypeFor[time.Time]()

	beforeInsertHookType = reflect.TypeFor[interface {
		BeforeInsert(context.Context) error
	}]()
	afterInsertHookType = reflect.TypeFor[interface {
		AfterInsert(context.Context) error
	}]()
	beforeUpdateHookType = reflect.TypeFor[interface {
		BeforeUpdate(context.Context) error
	}]()
	afterUpdateHookType = reflect.TypeFor[interface {
		AfterUpdate(context.Context) error
	}]()
	beforeDeleteHookType = reflect.TypeFor[interface {
		BeforeDelete(context.Context) error
	}]()
	afterDeleteHookType = reflect.TypeFor[interface {
		AfterDelete(context.Context) error
	}]()
	afterScanHookType = reflect.TypeFor[interface {
		AfterScan(context.Context) error
	}]()
)

var registry sync.Map // map[reflect.Type]func() (*Table, error)

func For(t reflect.Type) (*Table, error) {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	if v, ok := registry.Load(t); ok {
		return v.(func() (*Table, error))()
	}

	once := sync.OnceValues(func() (*Table, error) { return build(t) })
	actual, _ := registry.LoadOrStore(t, once)

	return actual.(func() (*Table, error))()
}

type buildCtx struct {
	structName string
	errs       []error
}

func (b *buildCtx) fail(err error) {
	b.errs = append(b.errs, err)
}

func build(t reflect.Type) (*Table, error) {
	c := &buildCtx{structName: t.Name()}

	table := &Table{
		GoType:         t,
		FieldsByColumn: map[string]*Field{},
	}

	var structOptions StructOptions
	var haveStructOpts bool

	walkFields(c, t, nil, table, &structOptions, &haveStructOpts)
	resolveTableName(c, t, table, structOptions)
	if structOptions.HasAlias {
		table.Alias = structOptions.Alias
	} else {
		table.Alias = table.Name
	}

	expandCompositeKeys(c, table)
	resolveRelationFKs(table)
	detectHooks(c, t, table)

	for i := range table.Fields {
		f := &table.Fields[i]
		table.FieldsByColumn[f.Column] = f
		if f.Options.PK {
			table.PK = append(table.PK, *f)
		}
	}

	if len(c.errs) > 0 {
		return nil, errors.Join(c.errs...)
	}

	return table, nil
}

func walkFields(c *buildCtx, t reflect.Type, indexPrefix []int, table *Table, structOpts *StructOptions, haveStructOpts *bool) {
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		index := appendIndex(indexPrefix, i)

		if sf.Anonymous {
			ft := sf.Type
			if ft.Kind() == reflect.Pointer {
				c.fail(&ErrPointerEmbed{Struct: c.structName, Field: sf.Name})
				continue
			}

			if raw, ok := sf.Tag.Lookup(packStructTag); ok {
				opts, err := ParseStructTag(c.structName, raw)
				if err != nil {
					c.fail(err)
				} else if !*haveStructOpts {
					*structOpts = opts
					*haveStructOpts = true
				}
			}

			if ft.Kind() == reflect.Struct {
				walkFields(c, ft, index, table, structOpts, haveStructOpts)
			}
			continue
		}

		if sf.PkgPath != "" {
			// Unexported, skip without any errors
			continue
		}

		raw, _ := sf.Tag.Lookup(packStructTag)
		tag, err := ParseFieldTag(c.structName, sf.Name, raw)
		if err != nil {
			c.fail(err)
			continue
		}

		if tag.Kind == KindRelation {
			rel, ok := buildRelation(c, sf, index, tag)
			if ok {
				table.Relations = append(table.Relations, rel)
			}
		} else {
			if tag.Column.Skip {
				continue
			}

			column := tag.NameSlot
			if column == "" {
				column = toSnakeCase(sf.Name)
			}
			table.Fields = append(table.Fields, Field{
				GoName:  sf.Name,
				Column:  column,
				Index:   index,
				Type:    sf.Type,
				Options: tag.Column,
			})
		}

	}
}

func resolveTableName(c *buildCtx, t reflect.Type, table *Table, structOpts StructOptions) {
	if name, ok := tableNameFromMethod(t); ok {
		table.Name = name
		return
	}

	if structOpts.HasTable {
		table.Name = structOpts.Table
		return
	}
	c.fail(&ErrMissingTableName{Struct: c.structName})
}

func tableNameFromMethod(t reflect.Type) (string, bool) {
	if t.Implements(tableNamerType) {
		v := reflect.New(t).Elem().Interface().(interface{ TableName() string })
		return v.TableName(), true
	}

	pt := reflect.PointerTo(t)
	if pt.Implements(tableNamerType) {
		v := reflect.New(t).Interface().(interface{ TableName() string })
		return v.TableName(), true
	}

	return "", false
}

func expandCompositeKeys(c *buildCtx, table *Table) {
	var expaned []Field

	for _, f := range table.Fields {
		if !f.Options.PK || !isCompositeKeyCandidate(f.Type) {
			expaned = append(expaned, f)
			continue
		}

		table.PKIsComposite = true
		ft := f.Type
		for i := 0; i < ft.NumField(); i++ {
			sf := ft.Field(i)
			if sf.PkgPath != "" {
				continue
			}

			if sf.Type.Kind() == reflect.Struct || sf.Type.Kind() == reflect.Pointer || sf.Type.Kind() == reflect.Slice {
				c.fail(&ErrCompositeKeyShape{
					Struct: c.structName, Field: f.GoName,
					Reason: "composite key fields must be flat scalars; found " + sf.Type.Kind().String() + " field " + sf.Name,
				})
				continue
			}

			raw, _ := sf.Tag.Lookup("db")
			tag, err := ParseFieldTag(ft.Name(), sf.Name, raw)
			if err != nil {
				c.fail(err)
				continue
			}

			column := tag.NameSlot
			if column == "" {
				column = toSnakeCase(sf.Name)
			}
			expaned = append(expaned, Field{
				GoName:  sf.Name,
				Column:  column,
				Index:   appendIndex(f.Index, i),
				Type:    sf.Type,
				Options: ColumnOptions{PK: true},
			})
		}
	}

	table.Fields = expaned
}

func isCompositeKeyCandidate(t reflect.Type) bool {
	if t.Kind() != reflect.Struct {
		return false
	}

	if t == timeType {
		return false
	}
	if t.Implements(valuerType) || reflect.PointerTo(t).Implements(valuerType) {
		return false
	}
	if t.Implements(scannerType) || reflect.PointerTo(t).Implements(scannerType) {
		return false
	}

	return true
}

func buildRelation(c *buildCtx, sf reflect.StructField, index []int, tag ParsedTag) (Relation, bool) {
	rel := Relation{
		GoName: sf.Name,
		Index:  index,
		Kind:   tag.Relation.Kind,
		FK:     tag.Relation.FK,
		Ref:    tag.Relation.Ref,
	}

	ft := sf.Type
	switch tag.Relation.Kind {
	case BelongsTo, HasOne:
		if ft.Kind() != reflect.Pointer || ft.Elem().Kind() != reflect.Struct {
			c.fail(&ErrRelationFieldShape{
				Struct: c.structName, Field: sf.Name, Kind: tag.Relation.Kind,
				Reason: "expected a pointer to struct",
			})
			return Relation{}, false
		}
		rel.TargetType = ft.Elem()
		rel.Slice = false
	case HasMany:
		if ft.Kind() != reflect.Slice || ft.Elem().Kind() != reflect.Pointer || ft.Elem().Elem().Kind() != reflect.Struct {
			c.fail(&ErrRelationFieldShape{
				Struct: c.structName, Field: sf.Name, Kind: tag.Relation.Kind,
				Reason: "expected a slice of pointer to struct",
			})
			return Relation{}, false
		}
		rel.TargetType = ft.Elem().Elem()
		rel.Slice = true
	}

	return rel, true
}

func detectHooks(c *buildCtx, t reflect.Type, table *Table) {
	table.Hooks.BeforeInsert = detectHook(c, t, beforeInsertHookType, "BeforeInsert")
	table.Hooks.AfterInsert = detectHook(c, t, afterInsertHookType, "AfterInsert")
	table.Hooks.BeforeUpdate = detectHook(c, t, beforeUpdateHookType, "BeforeUpdate")
	table.Hooks.AfterUpdate = detectHook(c, t, afterUpdateHookType, "AfterUpdate")
	table.Hooks.BeforeDelete = detectHook(c, t, beforeDeleteHookType, "BeforeDelete")
	table.Hooks.AfterDelete = detectHook(c, t, afterDeleteHookType, "AfterDelete")
	table.Hooks.AfterScan = detectHook(c, t, afterScanHookType, "AfterScan")
}

func detectHook(c *buildCtx, t, hookType reflect.Type, methodName string) bool {
	if t.Implements(hookType) {
		c.fail(&ErrValueReceiverHook{Struct: c.structName, Method: methodName})
		return false
	}

	return reflect.PointerTo(t).Implements(hookType)
}

func appendIndex(prefix []int, i int) []int {
	idx := make([]int, len(prefix)+1)
	copy(idx, prefix)
	idx[len(prefix)] = i
	return idx
}

func resolveRelationFKs(table *Table) {
	for i := range table.Relations {
		rel := &table.Relations[i]
		if rel.FK != "" {
			continue
		}

		switch rel.Kind {
		case BelongsTo:
			rel.FK = toSnakeCase(rel.TargetType.Name()) + "_id"
		case HasOne, HasMany:
			rel.FK = toSnakeCase(table.GoType.Name()) + "_id"
		}
	}
}
