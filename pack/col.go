package pack

import (
	"fmt"
	"reflect"
	"unsafe"

	"github.com/asimmons91/trails/pack/internal/schema"
	"github.com/asimmons91/trails/pack/internal/sqlbuild"
)

// Col[T, F] is a type-safe reference to one column of model T holding Go
// values of type F, produced by Field. Its methods build Predicates,
// OrderTerms, and Assignments against that column.
type Col[T, F any] struct {
	col sqlbuild.Column
}

// OrderTerm[T] is one ORDER BY term for T, produced by a Col's Asc/Desc.
type OrderTerm[T any] struct {
	o sqlbuild.OrderTerm
}

// Assignment is a column=value pair for an update, produced by a Col's Set
// or SetExpr.
type Assignment struct {
	a sqlbuild.Assignment
}

// Eq builds a "col = v" predicate.
func (c Col[T, F]) Eq(v F) Predicate { return Predicate{p: sqlbuild.Eq(c.col, v)} }

// Ne builds a "col <> v" predicate.
func (c Col[T, F]) Ne(v F) Predicate { return Predicate{p: sqlbuild.Ne(c.col, v)} }

// Gt builds a "col > v" predicate.
func (c Col[T, F]) Gt(v F) Predicate { return Predicate{p: sqlbuild.Gt(c.col, v)} }

// Gte builds a "col >= v" predicate.
func (c Col[T, F]) Gte(v F) Predicate { return Predicate{p: sqlbuild.Gte(c.col, v)} }

// Lt builds a "col < v" predicate.
func (c Col[T, F]) Lt(v F) Predicate { return Predicate{p: sqlbuild.Lt(c.col, v)} }

// Lte builds a "col <= v" predicate.
func (c Col[T, F]) Lte(v F) Predicate { return Predicate{p: sqlbuild.Lte(c.col, v)} }

// In builds a "col IN (vs...)" predicate.
func (c Col[T, F]) In(vs ...F) Predicate {
	return Predicate{p: sqlbuild.In(c.col, toAnySlice(vs)...)}
}

// NotIn builds a "col NOT IN (vs...)" predicate.
func (c Col[T, F]) NotIn(vs ...F) Predicate {
	return Predicate{p: sqlbuild.NotIn(c.col, toAnySlice(vs)...)}
}

// IsNull builds a "col IS NULL" predicate.
func (c Col[T, F]) IsNull() Predicate { return Predicate{p: sqlbuild.IsNull(c.col)} }

// IsNotNull builds a "col IS NOT NULL" predicate.
func (c Col[T, F]) IsNotNull() Predicate { return Predicate{p: sqlbuild.IsNotNull(c.col)} }

// Between builds a "col BETWEEN lo AND hi" predicate.
func (c Col[T, F]) Between(lo, hi F) Predicate {
	return Predicate{p: sqlbuild.Between(c.col, lo, hi)}
}

// Asc orders by this column ascending.
func (c Col[T, F]) Asc() OrderTerm[T] { return OrderTerm[T]{o: sqlbuild.Asc(c.col)} }

// Desc orders by this column descending.
func (c Col[T, F]) Desc() OrderTerm[T] { return OrderTerm[T]{o: sqlbuild.Desc(c.col)} }

// Set builds a "col = v" assignment for Update.
func (c Col[T, F]) Set(v F) Assignment { return Assignment{a: sqlbuild.Set(c.col, v)} }

// SetExpr builds a "col = <sqlExpr>" assignment for Update, with sqlExpr's
// own $-numbered placeholders bound to args — e.g. SetExpr("col + $1", 1)
// for an increment.
func (c Col[T, F]) SetExpr(sqlExpr string, args ...any) Assignment {
	return Assignment{a: sqlbuild.SetExpr(c.col, sqlExpr, args...)}
}

func (c Col[T, F]) sqlColumn() sqlbuild.Column { return c.col }

// AnyCol[T] is the type-erased form of Col[T, F], letting Select/GroupBy
// accept a slice of columns of T even when their F types differ.
type AnyCol[T any] interface {
	sqlColumn() sqlbuild.Column
}

// Field builds a Col[T, F] for the struct field sel selects, e.g.
// Field(func(a *Account) *string { return &a.Email }). It resolves sel's
// field by its runtime memory offset and matches that against T's mapped
// schema.Table, so it works for any exported, tagged field — this is the
// mechanism pack/fieldsgen-generated column accessors (e.g.
// accountCol.Email) are built on. Panics if T isn't a valid pack model or
// the selected field isn't one of its mapped columns.
func Field[T, F any](sel func(*T) *F) Col[T, F] {
	var zero T
	fptr := sel(&zero)
	table, err := schema.For(reflect.TypeFor[T]())
	if err != nil {
		panic(fmt.Sprintf("pack: Field[%T]: %v", zero, err))
	}

	offset := uintptr(unsafe.Pointer(fptr)) - uintptr(unsafe.Pointer(&zero))
	f := findFieldByOffset(table, offset)
	if f == nil {
		panic(fmt.Sprintf(
			"pack: Field selector for %T does not resolve to a mapped field at offset %d",
			zero, offset,
		))
	}

	return Col[T, F]{col: sqlbuild.QualifiedCol(table.Alias, f.Column)}
}

func findFieldByOffset(table *schema.Table, offset uintptr) *schema.Field {
	for i := range table.Fields {
		f := &table.Fields[i]
		if fieldOffset(table.GoType, f.Index) == offset {
			return f
		}
	}

	return nil
}

func fieldOffset(t reflect.Type, index []int) uintptr {
	var off uintptr
	cur := t
	for _, i := range index {
		sf := cur.Field(i)
		off += sf.Offset
		cur = sf.Type
	}

	return off
}

func toAnySlice[F any](vs []F) []any {
	out := make([]any, len(vs))
	for i, v := range vs {
		out[i] = v
	}

	return out
}
