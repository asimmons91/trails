package schema

import "fmt"

// ErrAmbiguousColumnSlot means a tag's name slot holds text that's also a
// recognized bare option (e.g. `db:"pk"`), so it's unclear whether "pk" is
// meant as the column name or the pk option.
type ErrAmbiguousColumnSlot struct {
	Struct  string
	Field   string
	Element string
}

func (e *ErrAmbiguousColumnSlot) Error() string {
	return fmt.Sprintf(
		"pack: %s.%s: tag element %q is a recognized option but sits in the column-name slot (no leading comma) — write `db:\",%s\"` to apply it as an option, or `db:\"%s,\"` to name the column %q deliberately",
		e.Struct, e.Field, e.Element, e.Element, e.Element, e.Element,
	)
}

// ErrMisplacedRelPrefix means a `rel:` element appeared somewhere other
// than the tag's first position.
type ErrMisplacedRelPrefix struct {
	Struct  string
	Field   string
	Element string
	Index   int
}

func (e *ErrMisplacedRelPrefix) Error() string {
	return fmt.Sprintf(
		"pack: %s.%s: `rel:` must be the tag's first element; found %q at position %d. Relation tags take no leading comma: write `db:\"rel:<kind>,...\"`",
		e.Struct, e.Field, e.Element, e.Index,
	)
}

// ErrUnknownRelationKind means a `rel:<kind>` element named a kind other
// than belongs_to, has_one, or has_many.
type ErrUnknownRelationKind struct {
	Struct string
	Field  string
	Kind   string
}

func (e *ErrUnknownRelationKind) Error() string {
	return fmt.Sprintf(
		"db: %s.%s: unknown relation kind %q; recognized kinds are belongs_to, has_one, has_many",
		e.Struct, e.Field, e.Kind,
	)
}

// ErrOptionWrongGrammar means a relation-only option (fk:/ref:) appeared on
// a column tag, or a column-only option (a bare flag, default:, type:)
// appeared on a relation tag.
type ErrOptionWrongGrammar struct {
	Struct   string
	Field    string
	Option   string
	Expected string
}

func (e *ErrOptionWrongGrammar) Error() string {
	return fmt.Sprintf(
		"pack: %s.%s: option %q is not valid on a %s tag",
		e.Struct, e.Field, e.Option, e.Expected,
	)
}

// ErrUnknownOption means a tag element matched none of the recognized
// column, relation, or struct options.
type ErrUnknownOption struct {
	Struct string
	Field  string
	Option string
}

func (e *ErrUnknownOption) Error() string {
	if e.Field == "" {
		return fmt.Sprintf("pack: %s: unknown option %q", e.Struct, e.Option)
	}

	return fmt.Sprintf("pack: %s.%s: unknown option %q", e.Struct, e.Field, e.Option)
}

// ErrFieldOptionOnStructTag means a column- or relation-level option
// appeared on an embedded field's struct-level tag, which only accepts
// table:/alias:.
type ErrFieldOptionOnStructTag struct {
	Struct string
	Option string
}

func (e *ErrFieldOptionOnStructTag) Error() string {
	return fmt.Sprintf(
		"pack: %s: option %q is a field-level option and is not valid on the struct-level tag",
		e.Struct, e.Option,
	)
}

// ErrStructOptionOnFieldTag means a struct-level option (table:/alias:)
// appeared on an ordinary field's tag instead of an embedded field's.
type ErrStructOptionOnFieldTag struct {
	Struct string
	Field  string
	Option string
}

func (e *ErrStructOptionOnFieldTag) Error() string {
	return fmt.Sprintf(
		"pack: %s.%s: option %q is a struct-level option and is not valid on a field tag",
		e.Struct, e.Field, e.Option,
	)
}

// ErrMissingTableName means a model has neither a TableName() string
// method nor a `table:` option on its embedded tag, so For has no way to
// name its table.
type ErrMissingTableName struct {
	Struct string
}

func (e *ErrMissingTableName) Error() string {
	return fmt.Sprintf(
		"pack: %s: no table name: define a TableName() string method or set `table:<name>` on the embedded model tag",
		e.Struct,
	)
}

// ErrPointerEmbed means a model embeds a pointer-typed field, which For
// doesn't support — embedded fields must be embedded by value.
type ErrPointerEmbed struct {
	Struct string
	Field  string
}

func (e *ErrPointerEmbed) Error() string {
	return fmt.Sprintf(
		"pack: %s.%s: embedded fields must not be pointers",
		e.Struct, e.Field,
	)
}

// ErrCompositeKeyShape means a struct-shaped PK field (see
// expandCompositeKeys) has a subfield that isn't a flat scalar.
type ErrCompositeKeyShape struct {
	Struct string
	Field  string
	Reason string
}

func (e *ErrCompositeKeyShape) Error() string {
	return fmt.Sprintf(
		"pack: %s.%s: invalid composite key type: %s",
		e.Struct, e.Field, e.Reason,
	)
}

// ErrRelationFieldShape means a relation field's Go type doesn't match its
// tagged Kind — belongs_to/has_one need *Target, has_many needs []*Target.
type ErrRelationFieldShape struct {
	Struct string
	Field  string
	Kind   RelationKind
	Reason string
}

func (e *ErrRelationFieldShape) Error() string {
	return fmt.Sprintf(
		"pack: %s.%s: invalid shape for %s relation: %s",
		e.Struct, e.Field, e.Kind, e.Reason,
	)
}

// ErrValueReceiverHook means a model implements a lifecycle hook method
// with a value receiver, whose mutations wouldn't be visible to the
// caller — the hook must use a pointer receiver.
type ErrValueReceiverHook struct {
	Struct string
	Method string
}

func (e *ErrValueReceiverHook) Error() string {
	return fmt.Sprintf(
		"pack: %s.%s must use a pointer receiver (func (m *%s) %s(...)) — a value-receiver hook's mutations are silently discarded",
		e.Struct, e.Method, e.Struct, e.Method,
	)
}
