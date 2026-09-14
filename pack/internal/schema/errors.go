package schema

import "fmt"

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

type ErrMissingTableName struct {
	Struct string
}

func (e *ErrMissingTableName) Error() string {
	return fmt.Sprintf(
		"pack: %s: no table name: define a TableName() string method or set `table:<name>` on the embedded model tag",
		e.Struct,
	)
}

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
