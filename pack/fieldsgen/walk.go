package fieldsgen

import (
	"fmt"
	"go/types"
	"reflect"

	"github.com/asimmons91/trails/pack/internal/schema"
)

// walker flattens a discovered model's *types.Struct into Col/Rel
// descriptors, mirroring pack/internal/schema.walkFields but over go/types
// instead of reflect, and reusing schema.ParseFieldTag directly for tag
// grammar so the two never drift apart.
type walker struct {
	aux auxTypes
}

// flattenResult is one model's flattened shape, plus any structural errors
// encountered while walking it.
type flattenResult struct {
	Cols    []ColField
	Rels    []RelField
	Skipped []SkippedField
	Errs    []error
}

func (w *walker) flatten(structName string, st *types.Struct) flattenResult {
	var res flattenResult
	w.walkFields(structName, st, &res)
	return res
}

func (w *walker) walkFields(structName string, st *types.Struct, res *flattenResult) {
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)

		if f.Embedded() {
			w.walkEmbedded(structName, f, res)
			continue
		}

		if !f.Exported() {
			continue
		}

		raw, _ := reflect.StructTag(st.Tag(i)).Lookup("db")
		tag, err := schema.ParseFieldTag(structName, f.Name(), raw)
		if err != nil {
			res.Errs = append(res.Errs, err)
			continue
		}

		if tag.Kind == schema.KindRelation {
			if rel, ok := w.buildRelation(structName, f, tag, res); ok {
				res.Rels = append(res.Rels, rel)
			}
			continue
		}

		if tag.Column.Skip {
			continue
		}

		if tag.Column.PK && w.isCompositeKeyCandidate(f.Type()) {
			res.Skipped = append(res.Skipped, SkippedField{
				GoName: f.Name(),
				Reason: fmt.Sprintf(
					"composite primary key (%s); a single pack.Field selector cannot represent a multi-column key — see pack.Field's offset-resolution mechanics in pack/col.go",
					typeName(f.Type()),
				),
			})
			continue
		}

		res.Cols = append(res.Cols, ColField{GoName: f.Name(), Type: f.Type()})
	}
}

func (w *walker) walkEmbedded(structName string, f *types.Var, res *flattenResult) {
	ft := f.Type()

	if _, ok := ft.(*types.Pointer); ok {
		res.Errs = append(res.Errs, &schema.ErrPointerEmbed{Struct: structName, Field: f.Name()})
		return
	}

	named, ok := ft.(*types.Named)
	if !ok {
		return
	}

	inner, ok := named.Underlying().(*types.Struct)
	if !ok {
		return
	}

	w.walkFields(structName, inner, res)
}

func (w *walker) buildRelation(structName string, f *types.Var, tag schema.ParsedTag, res *flattenResult) (RelField, bool) {
	shapeErr := func(reason string) {
		res.Errs = append(res.Errs, &schema.ErrRelationFieldShape{
			Struct: structName, Field: f.Name(), Kind: tag.Relation.Kind, Reason: reason,
		})
	}

	switch tag.Relation.Kind {
	case schema.BelongsTo, schema.HasOne:
		ptr, ok := f.Type().(*types.Pointer)
		if !ok {
			shapeErr("expected a pointer to struct")
			return RelField{}, false
		}
		target, ok := ptr.Elem().(*types.Named)
		if !ok {
			shapeErr("expected a pointer to struct")
			return RelField{}, false
		}
		return RelField{GoName: f.Name(), Kind: RelOne, Target: target}, true

	case schema.HasMany:
		slice, ok := f.Type().(*types.Slice)
		if !ok {
			shapeErr("expected a slice of pointer to struct")
			return RelField{}, false
		}
		elemPtr, ok := slice.Elem().(*types.Pointer)
		if !ok {
			shapeErr("expected a slice of pointer to struct")
			return RelField{}, false
		}
		target, ok := elemPtr.Elem().(*types.Named)
		if !ok {
			shapeErr("expected a slice of pointer to struct")
			return RelField{}, false
		}
		return RelField{GoName: f.Name(), Kind: RelMany, Target: target}, true
	}

	return RelField{}, false
}

// isCompositeKeyCandidate mirrors
// pack/internal/schema.isCompositeKeyCandidate: a PK field whose Go type is
// itself a plain struct (not time.Time, not a driver.Valuer/sql.Scanner)
// gets expanded into multiple PK columns at runtime by schema.build, which
// a single pack.Field selector cannot represent.
func (w *walker) isCompositeKeyCandidate(t types.Type) bool {
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	if _, ok := named.Underlying().(*types.Struct); !ok {
		return false
	}

	if w.aux.timeType != nil && types.Identical(named, w.aux.timeType) {
		return false
	}
	if w.aux.valuerIface != nil &&
		(types.Implements(named, w.aux.valuerIface) || types.Implements(types.NewPointer(named), w.aux.valuerIface)) {
		return false
	}
	if w.aux.scannerIface != nil &&
		(types.Implements(named, w.aux.scannerIface) || types.Implements(types.NewPointer(named), w.aux.scannerIface)) {
		return false
	}

	return true
}

func typeName(t types.Type) string {
	if named, ok := t.(*types.Named); ok {
		return named.Obj().Name()
	}
	return t.String()
}
