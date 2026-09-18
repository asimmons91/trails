package schema

import "strings"

// FieldKind says whether a parsed `db:"..."` tag describes a column or a
// relation.
type FieldKind int

const (
	KindColumn FieldKind = iota
	KindRelation
)

// RelationKind is the kind of relation a `db:"rel:<kind>"` tag names.
type RelationKind int

const (
	BelongsTo RelationKind = iota
	HasOne
	HasMany
)

// String renders a RelationKind as it appears in tag syntax and error
// messages ("belongs_to", "has_one", "has_many").
func (r RelationKind) String() string {
	switch r {
	case BelongsTo:
		return "belongs_to"
	case HasOne:
		return "has_one"
	case HasMany:
		return "has_many"
	default:
		return "unknown"
	}
}

// ColumnOptions are the parsed option flags from a column tag, e.g.
// `db:"email,unique,not_null"`.
type ColumnOptions struct {
	PK            bool
	AutoIncrement bool
	// NullZero treats the field's zero value as SQL NULL on write.
	NullZero   bool
	Unique     bool
	NotNull    bool
	Default    string
	HasDefault bool
	// SQLType overrides the column's inferred SQL type, from a `type:`
	// option.
	SQLType string
	// Skip marks a field excluded from the table entirely, from a `db:"-"`
	// tag.
	Skip bool
}

// RelationOptions are the parsed options from a relation tag, e.g.
// `db:"rel:belongs_to,fk:author_id"`.
type RelationOptions struct {
	Kind RelationKind
	// FK and Ref are the foreign-key and referenced-key column names. Left
	// empty, resolveRelationFKs fills FK in by convention.
	FK  string
	Ref string
}

// ParsedTag is the result of parsing one field's `db:"..."` tag: either a
// column (Column populated, Kind == KindColumn) or a relation (Relation
// populated, Kind == KindRelation) — never both.
type ParsedTag struct {
	Kind     FieldKind
	Column   ColumnOptions
	Relation RelationOptions
	// NameSlot is the column tag's leading name element (e.g. "email" in
	// `db:"email,unique"`), empty if left blank for the default
	// snake_case name.
	NameSlot string
	// HasLeadingComma is true for a deliberately empty NameSlot
	// (`db:",unique"`), distinguishing "use the default name" from "the
	// name slot happened to be empty."
	HasLeadingComma bool
}

// StructOptions are the parsed options from a struct-level tag on an
// embedded field, e.g. `db:"table:users,alias:u"`.
type StructOptions struct {
	Table    string
	HasTable bool
	Alias    string
	HasAlias bool
}

var bareColumnOptions = map[string]bool{
	"pk":             true,
	"auto_increment": true,
	"null_zero":      true,
	"unique":         true,
	"not_null":       true,
}

var prefixColumnOptions = []string{"default:", "type:", "fk:", "ref:"}

// ParseFieldTag parses one struct field's `db:"..."` tag value raw into a
// ParsedTag. `db:"-"` skips the field entirely. A tag whose first
// comma-separated element starts with "rel:" is parsed as a relation
// (`db:"rel:<kind>[,fk:...][,ref:...]"`); anything else is parsed as a
// column (`db:"[name][,opt]..."`, options from bareColumnOptions or a
// `default:`/`type:` prefix). "rel:" is only recognized as the tag's first
// element — found elsewhere, it's an error (ErrMisplacedRelPrefix).
func ParseFieldTag(structName, fieldName, raw string) (ParsedTag, error) {
	if raw == "-" {
		return ParsedTag{Kind: KindColumn, Column: ColumnOptions{Skip: true}}, nil
	}

	elems := strings.Split(raw, ",")
	for i, e := range elems {
		if strings.HasPrefix(e, "rel:") && i != 0 {
			return ParsedTag{}, &ErrMisplacedRelPrefix{
				Struct: structName, Field: fieldName, Element: e, Index: i,
			}
		}
	}

	if strings.HasPrefix(elems[0], "rel:") {
		return parseRelationTag(structName, fieldName, elems)
	}

	return parseColumnTag(structName, fieldName, raw, elems)
}

// ParseStructTag parses the `db:"..."` tag value raw found on an embedded
// field into a StructOptions, recognizing only `table:` and `alias:`.
// Any column- or relation-only option in raw is an error
// (ErrFieldOptionOnStructTag), as is anything unrecognized
// (ErrUnknownOption).
func ParseStructTag(structName, raw string) (StructOptions, error) {
	var opts StructOptions
	if raw == "" {
		return opts, nil
	}

	for elem := range strings.SplitSeq(raw, ",") {
		if elem == "" {
			continue
		}

		switch {
		case strings.HasPrefix(elem, "table:"):
			opts.Table = strings.TrimPrefix(elem, "table:")
			opts.HasTable = true
		case strings.HasPrefix(elem, "alias:"):
			opts.Alias = strings.TrimPrefix(elem, "alias:")
			opts.HasAlias = true
		case bareColumnOptions[elem] || elem == "-" ||
			strings.HasPrefix(elem, "default:") || strings.HasPrefix(elem, "type:") ||
			strings.HasPrefix(elem, "fk:") || strings.HasPrefix(elem, "ref:") ||
			strings.HasPrefix(elem, "rel:"):
			return StructOptions{}, &ErrFieldOptionOnStructTag{Struct: structName, Option: elem}
		default:
			return StructOptions{}, &ErrUnknownOption{Struct: structName, Option: elem}
		}
	}

	return opts, nil
}

func parseRelationTag(structName, fieldName string, elems []string) (ParsedTag, error) {
	kindStr := strings.TrimPrefix(elems[0], "rel:")
	var kind RelationKind

	switch kindStr {
	case "belongs_to":
		kind = BelongsTo
	case "has_one":
		kind = HasOne
	case "has_many":
		kind = HasMany
	default:
		return ParsedTag{}, &ErrUnknownRelationKind{Struct: structName, Field: fieldName, Kind: kindStr}

	}

	tag := ParsedTag{Kind: KindRelation, Relation: RelationOptions{Kind: kind}}

	for _, opt := range elems[1:] {
		if opt == "" {
			continue
		}

		switch {
		case strings.HasPrefix(opt, "fk:"):
			tag.Relation.FK = strings.TrimPrefix(opt, "fk:")
		case strings.HasPrefix(opt, "ref:"):
			tag.Relation.Ref = strings.TrimPrefix(opt, "ref:")
		case bareColumnOptions[opt], strings.HasPrefix(opt, "default:"), strings.HasPrefix(opt, "type:"):
			return ParsedTag{}, &ErrOptionWrongGrammar{
				Struct: structName, Field: fieldName, Option: opt, Expected: "relation",
			}
		case strings.HasPrefix(opt, "table:"), strings.HasPrefix(opt, "alias:"):
			return ParsedTag{}, &ErrStructOptionOnFieldTag{Struct: structName, Field: fieldName, Option: opt}
		default:
			return ParsedTag{}, &ErrUnknownOption{Struct: structName, Field: fieldName, Option: opt}
		}
	}

	return tag, nil
}

func parseColumnTag(structName, fieldName, raw string, elems []string) (ParsedTag, error) {
	if raw == "" {
		return ParsedTag{Kind: KindColumn}, nil
	}

	hasLeadingComma := elems[0] == ""
	isDeliberateEscape := len(elems) == 2 && elems[1] == ""
	slot := elems[0]

	if isOptionLike(slot) && !isDeliberateEscape {
		return ParsedTag{}, &ErrAmbiguousColumnSlot{
			Struct: structName, Field: fieldName, Element: slot,
		}
	}

	tag := ParsedTag{Kind: KindColumn, NameSlot: slot, HasLeadingComma: hasLeadingComma}

	var opts []string
	if isDeliberateEscape {
		opts = nil
	} else {
		opts = elems[1:]
	}

	for _, opt := range opts {
		if opt == "" {
			continue
		}

		switch {
		case opt == "pk":
			tag.Column.PK = true
		case opt == "auto_increment":
			tag.Column.AutoIncrement = true
		case opt == "null_zero":
			tag.Column.NullZero = true
		case opt == "unique":
			tag.Column.Unique = true
		case opt == "not_null":
			tag.Column.NotNull = true
		case strings.HasPrefix(opt, "default:"):
			tag.Column.Default = strings.TrimPrefix(opt, "default:")
			tag.Column.HasDefault = true
		case strings.HasPrefix(opt, "type:"):
			tag.Column.SQLType = strings.TrimPrefix(opt, "type:")
		case strings.HasPrefix(opt, "fk:"), strings.HasPrefix(opt, "ref:"):
			return ParsedTag{}, &ErrOptionWrongGrammar{
				Struct: structName, Field: fieldName, Option: opt, Expected: "column",
			}
		case strings.HasPrefix(opt, "table:"), strings.HasPrefix(opt, "alias:"):
			return ParsedTag{}, &ErrStructOptionOnFieldTag{Struct: structName, Field: fieldName, Option: opt}
		default:
			return ParsedTag{}, &ErrUnknownOption{Struct: structName, Field: fieldName, Option: opt}
		}
	}

	return tag, nil
}

func isOptionLike(elem string) bool {
	if bareColumnOptions[elem] {
		return true
	}

	for _, p := range prefixColumnOptions {
		if strings.HasPrefix(elem, p) {
			return true
		}
	}

	return false
}
