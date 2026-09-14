package pack

import (
	"context"
	"fmt"
	"reflect"

	"github.com/asimmons91/trails/pack/internal/schema"
	"github.com/asimmons91/trails/pack/internal/sqlbuild"
)

type preloadRunner func(ctx context.Context, db *DB, parentsAny any) error

type PreloadOption interface {
	applyTo(spec *preloadSpec)
}

type preloadSpec struct {
	where  sqlbuild.Predicate
	nested []preloadRunner
}

type whereOption struct {
	p sqlbuild.Predicate
}

func (o whereOption) applyTo(s *preloadSpec) {
	s.where = andJoin(s.where, o.p)
}

func Where(p Predicate) PreloadOption {
	return whereOption(p)
}

type nestedOption struct{ run preloadRunner }

func (o nestedOption) applyTo(s *preloadSpec) { s.nested = append(s.nested, o.run) }

func Preload[T, U any](rel Rel[T, U], opts ...PreloadOption) PreloadOption {
	spec := &preloadSpec{}
	for _, o := range opts {
		o.applyTo(spec)
	}

	run := func(ctx context.Context, db *DB, parentsAny any) error {
		return runPreload[T, U](ctx, db, parentsAny.([]T), rel, spec)
	}
	return nestedOption{run}
}

func (q *Query[T]) Preload[U any](rel Rel[T, U], opts ...PreloadOption) *Query[T] {
	spec := &preloadSpec{}
	for _, o := range opts {
		o.applyTo(spec)
	}

	run := func(ctx context.Context, db *DB, parentsAny any) error {
		return runPreload[T, U](ctx, db, parentsAny.([]T), rel, spec)
	}

	nq := q.clone()
	nq.preloads = appendFresh(nq.preloads, run)
	return nq
}

func resolveRefColumn(rel *schema.Relation, ownerTable, targetTable *schema.Table) (string, error) {
	if rel.Ref != "" {
		return rel.Ref, nil
	}
	table := ownerTable

	if rel.Kind == schema.BelongsTo {
		table = targetTable
	}

	if table.PKIsComposite || len(table.PK) != 1 {
		return "", fmt.Errorf(
			"pack: relation %s: cannot default ref against a composite or missing primary key on %s; set ref: explicitly",
			rel.GoName, table.GoType.Name(),
		)
	}

	return table.PK[0].Column, nil
}

func runPreload[T, U any](ctx context.Context, db *DB, parents []T, rel Rel[T, U], spec *preloadSpec) error {
	if len(parents) == 0 {
		return nil
	}

	ownerTable := schemaForOrPanic[T]("Preload")
	targetTable := schemaForOrPanic[U]("Preload")

	refCol, err := resolveRefColumn(&rel.relation, ownerTable, targetTable)
	if err != nil {
		return err
	}

	var batchColumn, childWhereColumn string
	if rel.relation.Kind == schema.BelongsTo {
		batchColumn = rel.relation.FK
		childWhereColumn = refCol
	} else {
		batchColumn = refCol
		childWhereColumn = rel.relation.FK
	}

	batchField := ownerTable.FieldsByColumn[batchColumn]
	if batchField == nil {
		return fmt.Errorf("pack: relation %s: no field mapped for column %q on %s", rel.relation.GoName, batchColumn, ownerTable.GoType.Name())
	}

	childField := targetTable.FieldsByColumn[childWhereColumn]
	if childField == nil {
		return fmt.Errorf("pack: relation %s: no field mapped for column %q on %s", rel.relation.GoName, childWhereColumn, targetTable.GoType.Name())
	}

	pv := reflect.ValueOf(parents)
	keyToParents := map[any][]int{}
	var keys []any
	for i := 0; i < pv.Len(); i++ {
		v := pv.Index(i).FieldByIndex(batchField.Index)
		if v.IsZero() {
			continue
		}

		key := v.Interface()
		if _, seen := keyToParents[key]; !seen {
			keys = append(keys, key)
		}
		keyToParents[key] = append(keyToParents[key], i)
	}
	if len(keys) == 0 {
		return nil
	}

	childCol := sqlbuild.QualifiedCol(targetTable.Alias, childWhereColumn)
	where := sqlbuild.In(childCol, keys...)
	if !spec.where.IsZero() {
		where = sqlbuild.And(where, spec.where)
	}

	cols := make([]sqlbuild.Column, len(targetTable.Fields))
	for i, f := range targetTable.Fields {
		cols[i] = sqlbuild.QualifiedCol(targetTable.Alias, f.Column)
	}

	sqlText, args, err := sqlbuild.Select(sqlbuild.Table{
		Name:  targetTable.Name,
		Alias: targetTable.Alias,
	}).
		Columns(cols...).
		Where(where).
		Render(db.dialect)
	if err != nil {
		return err
	}

	rows, err := db.queryContext(ctx, "Preload", targetTable.GoType.Name(), sqlText, args)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	children, err := scanAllRows[U](ctx, rows, targetTable)
	if err != nil {
		return err
	}

	for _, nestedRun := range spec.nested {
		if err := nestedRun(ctx, db, children); err != nil {
			return err
		}
	}

	childrenByKey := map[any][]U{}
	for _, c := range children {
		v := reflect.ValueOf(c).FieldByIndex(childField.Index)
		key := v.Interface()
		childrenByKey[key] = append(childrenByKey[key], c)
	}

	for key, idxs := range keyToParents {
		kids := childrenByKey[key]
		if len(kids) == 0 {
			continue
		}

		for _, idx := range idxs {
			fieldVal := pv.Index(idx).FieldByIndex(rel.relation.Index)
			if rel.relation.Slice {
				ptrs := make([]*U, len(kids))
				for j := range kids {
					ptrs[j] = &kids[j]
				}
				fieldVal.Set(reflect.ValueOf(ptrs))
			} else {
				fieldVal.Set(reflect.ValueOf(&kids[0]))
			}
		}
	}

	return nil
}
