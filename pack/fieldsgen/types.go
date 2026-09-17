// Package fieldsgen statically analyzes a package of pack ORM models and
// generates the type-safe Col/Rel field-and-relation helper boilerplate that
// callers would otherwise hand-write per model (see pack.Field, pack.Relation
// and pack.RelationOne).
package fieldsgen

import "go/types"

// RelKind is the cardinality of a discovered relation field.
type RelKind int

const (
	// RelOne is a belongs_to or has_one relation (pack.RelationOne).
	RelOne RelKind = iota
	// RelMany is a has_many relation (pack.Relation).
	RelMany
)

// ColField is a single flattened, exported scalar column on a model.
type ColField struct {
	GoName string
	Type   types.Type
}

// RelField is a single flattened, exported relation on a model.
type RelField struct {
	GoName string
	Kind   RelKind
	Target *types.Named
}

// SkippedField documents a field the generator deliberately did not turn
// into a Col entry, along with why, so the reason ends up in the generated
// source as a comment instead of silently vanishing.
type SkippedField struct {
	GoName string
	Reason string
}

// ModelInfo is the flattened shape of one discovered model, ready to render.
type ModelInfo struct {
	Name    string
	Cols    []ColField
	Rels    []RelField
	Skipped []SkippedField
}

// Options configures a Generate call.
type Options struct {
	// Dir is the directory containing the model source files to analyze.
	Dir string
	// OutFile is the name of the generated file, written inside Dir.
	OutFile string
}

// Result summarizes a successful Generate call.
type Result struct {
	ModelCount int
	OutPath    string
}
