package migrate

import "fmt"

// ErrConstraintWrongRelationKind is returned by CreateConstraint when the
// named relation isn't a belongs_to — only a belongs_to relation has its FK
// column on this table, so it's the only kind CreateConstraint can build a
// constraint from.
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

// ErrRelationNotFound is returned when a Migrator method is asked to act on
// a relation name a model doesn't have.
type ErrRelationNotFound struct {
	Model    string
	Relation string
}

func (e *ErrRelationNotFound) Error() string {
	return fmt.Sprintf("pack/migrate: %s has no relation named %q", e.Model, e.Relation)
}

// ErrAlterColumnRequiresTopLevelConnection is returned by AlterColumn when
// the dialect requires a table rebuild (see dialect.ErrRequiresTableRebuild)
// but the Migrator's *pack.DB is already inside a transaction. The rebuild
// pins and manages its own connection and transaction, so it can't run
// nested inside a caller's.
var ErrAlterColumnRequiresTopLevelConnection = fmt.Errorf(
	"pack/migrate: AlterColumn on SQLite requires a table rebuild, which cannot run inside an existing transaction; call it on a top-level *pack.DB",
)

// ErrForeignKeyCheckFailed is returned by AlterColumn's rebuild path when
// PRAGMA foreign_key_check finds violations after rebuilding Table; Rows
// holds the raw rows the pragma reported.
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

// WithoutIfExists makes Drop{Table,Column,Index,Constraint} issue the DROP
// unconditionally instead of first checking whether it exists.
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

// IndexOption configures CreateIndex.
type IndexOption func(*indexConfig)

type indexConfig struct {
	unique bool
}

// WithUniqueIndex makes CreateIndex create a UNIQUE index.
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

// ConstraintOption configures CreateConstraint.
type ConstraintOption func(*constraintConfig)

type constraintConfig struct {
	name string
}

// WithConstraintName overrides CreateConstraint's default generated
// constraint name.
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
