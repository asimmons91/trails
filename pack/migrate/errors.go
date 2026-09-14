package migrate

import "fmt"

type ErrConstraintWrongRelationKind struct {
	Model    string
	Relation string
	Kind     string
}

func (e *ErrConstraintWrongRelationKind) Error() string {
	return fmt.Sprintf(
		"pack/migrate: CreateConstraint(%s, %q): relation kind %q cannot have its FK constraint created from this table; only belongs_to relations can (the FK column lives on the belongs_to side)",
		e.Model, e.Relation, e.Kind,
	)
}

type ErrRelationNotFound struct {
	Model    string
	Relation string
}

func (e *ErrRelationNotFound) Error() string {
	return fmt.Sprintf("pack/migrate: %s has no relation named %q", e.Model, e.Relation)
}

var ErrAlterColumnRequiresTopLevelConnection = fmt.Errorf(
	"pack/migrate: AlterColumn on SQLite requires a table rebuild, which cannot run inside an existing transaction; call it on a top-level *pack.DB",
)

type ErrForeignKeyCheckFailed struct {
	Table string
	Rows  []map[string]any
}

func (e *ErrForeignKeyCheckFailed) Error() string {
	return fmt.Sprintf("pack/migrate: PRAGMA foreign_key_check found %d violation(s) after rebuilding %q", len(e.Rows), e.Table)
}

// DropOption configures Drop{Table,Column,Index,Constraint}.
type DropOption func(*dropConfig)

type dropConfig struct {
	withoutIfExists bool
}

func WithoutIfExists() DropOption {
	return func(c *dropConfig) { c.withoutIfExists = true }
}

func applyDropOptions(opts []DropOption) dropConfig {
	var c dropConfig
	for _, o := range opts {
		o(&c)
	}
	return c
}

type IndexOption func(*indexConfig)

type indexConfig struct {
	unique bool
}

func WithUniqueIndex() IndexOption {
	return func(c *indexConfig) { c.unique = true }
}

func applyIndexOptions(opts []IndexOption) indexConfig {
	var c indexConfig
	for _, o := range opts {
		o(&c)
	}
	return c
}

type ConstraintOption func(*constraintConfig)

type constraintConfig struct {
	name string
}

func WithConstraintName(name string) ConstraintOption {
	return func(c *constraintConfig) { c.name = name }
}

func applyConstraintOptions(opts []ConstraintOption) constraintConfig {
	var c constraintConfig
	for _, o := range opts {
		o(&c)
	}
	return c
}
