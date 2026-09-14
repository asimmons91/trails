package pack

import (
	"fmt"
	"unsafe"

	"github.com/asimmons91/trails/pack/internal/schema"
)

type Rel[T, U any] struct {
	relation schema.Relation
}

func Relation[T, U any](sel func(*T) *[]*U) Rel[T, U] {
	var zero T
	fptr := sel(&zero)
	table := schemaForOrPanic[T]("Relation")

	offset := uintptr(unsafe.Pointer(fptr)) - uintptr(unsafe.Pointer(&zero))
	r := findRelationByOffset(table, offset)
	if r == nil {
		panic(fmt.Sprintf(
			"pack: Relation selector for %T does not resolve to a mapped has_many relation at offset %d",
			zero, offset,
		))
	}

	return Rel[T, U]{relation: *r}
}

func RelationOne[T, U any](sel func(*T) **U) Rel[T, U] {
	var zero T
	fptr := sel(&zero)
	table := schemaForOrPanic[T]("RelationOne")

	offset := uintptr(unsafe.Pointer(fptr)) - uintptr(unsafe.Pointer(&zero))
	r := findRelationByOffset(table, offset)
	if r == nil {
		panic(fmt.Sprintf(
			"pack: RelationOne selector for %T does not resolve to a mapped belongs_to/has_one relation at offset %d",
			zero, offset,
		))
	}

	return Rel[T, U]{relation: *r}
}

func findRelationByOffset(table *schema.Table, offset uintptr) *schema.Relation {
	for i := range table.Relations {
		if fieldOffset(table.GoType, table.Relations[i].Index) == offset {
			return &table.Relations[i]
		}
	}

	return nil
}
