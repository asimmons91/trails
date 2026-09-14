package pack

import (
	"fmt"
	"reflect"
	"unsafe"

	"github.com/asimmons91/trails/pack/internal/schema"
	"github.com/asimmons91/trails/pack/internal/sqlbuild"
)

type Col[T, F any] struct {
	col sqlbuild.Column
}

type OrderTerm[T any] struct {
	o sqlbuild.OrderTerm
}

type Assignment struct {
	a sqlbuild.Assignment
}

func (c Col[T, F]) Eq(v F) Predicate { return Predicate{p: sqlbuild.Eq(c.col, v)} }

func (c Col[T, F]) Ne(v F) Predicate { return Predicate{p: sqlbuild.Ne(c.col, v)} }

func (c Col[T, F]) Gt(v F) Predicate { return Predicate{p: sqlbuild.Gt(c.col, v)} }

func (c Col[T, F]) Gte(v F) Predicate { return Predicate{p: sqlbuild.Gte(c.col, v)} }

func (c Col[T, F]) Lt(v F) Predicate { return Predicate{p: sqlbuild.Lt(c.col, v)} }

func (c Col[T, F]) Lte(v F) Predicate { return Predicate{p: sqlbuild.Lte(c.col, v)} }

func (c Col[T, F]) In(vs ...F) Predicate {
	return Predicate{p: sqlbuild.In(c.col, toAnySlice(vs)...)}
}

func (c Col[T, F]) NotIn(vs ...F) Predicate {
	return Predicate{p: sqlbuild.NotIn(c.col, toAnySlice(vs)...)}
}

func (c Col[T, F]) IsNull() Predicate { return Predicate{p: sqlbuild.IsNull(c.col)} }

func (c Col[T, F]) IsNotNull() Predicate { return Predicate{p: sqlbuild.IsNotNull(c.col)} }

func (c Col[T, F]) Between(lo, hi F) Predicate {
	return Predicate{p: sqlbuild.Between(c.col, lo, hi)}
}

func (c Col[T, F]) Asc() OrderTerm[T] { return OrderTerm[T]{o: sqlbuild.Asc(c.col)} }

func (c Col[T, F]) Desc() OrderTerm[T] { return OrderTerm[T]{o: sqlbuild.Desc(c.col)} }

func (c Col[T, F]) Set(v F) Assignment { return Assignment{a: sqlbuild.Set(c.col, v)} }

func (c Col[T, F]) SetExpr(sqlExpr string, args ...any) Assignment {
	return Assignment{a: sqlbuild.SetExpr(c.col, sqlExpr, args...)}
}

func (c Col[T, F]) sqlColumn() sqlbuild.Column { return c.col }

type AnyCol[T any] interface {
	sqlColumn() sqlbuild.Column
}

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
