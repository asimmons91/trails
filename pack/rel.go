package pack

import (
	"fmt"
	"unsafe"

	"github.com/asimmons91/trails/pack/internal/schema"
)

// Rel[T, U] is a typed reference to a relation from model T to model U,
// produced by Relation or RelationOne and consumed by Query[T].Preload.
type Rel[T, U any] struct {
	relation schema.Relation
}

// Relation builds a Rel[T, U] for a has-many []*U field, e.g.
// Relation(func(a *Account) *[]*Post { return &a.Posts }). Like Field, it
// resolves sel's field by runtime offset. Panics if T isn't a valid pack
// model or the selected field isn't a mapped has-many relation.
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

// RelationOne builds a Rel[T, U] for a has-one/belongs-to *U field, e.g.
// RelationOne(func(p *Post) **Account { return &p.Author }). Panics under
// the same conditions as Relation.
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
