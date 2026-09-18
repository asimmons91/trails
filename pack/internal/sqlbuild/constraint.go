package sqlbuild

import (
	"strings"

	"github.com/asimmons91/trails/pack/dialect"
)

// CreateConstraintBuilder builds an ALTER TABLE ... ADD CONSTRAINT ...
// FOREIGN KEY statement. Build one with CreateForeignKeyConstraint.
type CreateConstraintBuilder struct {
	table     Table
	name      string
	localCols []string
	refTable  string
	refCols   []string
}

// CreateForeignKeyConstraint starts a CreateConstraintBuilder adding a
// foreign key named name on t, from localCols to refCols on refTable.
func CreateForeignKeyConstraint(t Table, name string, localCols []string, refTable string, refCols []string) *CreateConstraintBuilder {
	return &CreateConstraintBuilder{
		table:     t,
		name:      name,
		localCols: append([]string(nil), localCols...),
		refTable:  refTable,
		refCols:   append([]string(nil), refCols...),
	}
}

// Render renders b as ALTER TABLE ... ADD CONSTRAINT SQL for dialect d.
// It errors with ErrConstraintsUnsupportedByDialect on SQLite, which has
// no way to add a constraint to an existing table.
func (b *CreateConstraintBuilder) Render(d dialect.Dialect) (string, []any, error) {
	if d.Name() == "sqlite" {
		return "", nil, &ErrConstraintsUnsupportedByDialect{Dialect: d.Name()}
	}

	var sb strings.Builder
	sb.WriteString("ALTER TABLE ")
	sb.WriteString(d.QuoteIdent(b.table.Name))
	sb.WriteString(" ADD CONSTRAINT ")
	sb.WriteString(d.QuoteIdent(b.name))
	sb.WriteString(" FOREIGN KEY (")
	sb.WriteString(quoteIdentList(d, b.localCols))
	sb.WriteString(") REFERENCES ")
	sb.WriteString(d.QuoteIdent(b.refTable))
	sb.WriteString(" (")
	sb.WriteString(quoteIdentList(d, b.refCols))
	sb.WriteString(")")

	return sb.String(), nil, nil
}

// DropConstraintBuilder builds a statement dropping a named constraint.
// Build one with DropConstraint.
type DropConstraintBuilder struct {
	table Table
	name  string
}

// DropConstraint starts a DropConstraintBuilder dropping the constraint
// named name from t.
func DropConstraint(t Table, name string) *DropConstraintBuilder {
	return &DropConstraintBuilder{table: t, name: name}
}

// Render renders b as ALTER TABLE ... DROP CONSTRAINT SQL for dialect d
// (MySQL's own DROP FOREIGN KEY syntax on MySQL). It errors with
// ErrConstraintsUnsupportedByDialect on SQLite.
func (b *DropConstraintBuilder) Render(d dialect.Dialect) (string, []any, error) {
	if d.Name() == "sqlite" {
		return "", nil, &ErrConstraintsUnsupportedByDialect{Dialect: d.Name()}
	}

	verb := "DROP CONSTRAINT"
	if d.Name() == "mysql" {
		verb = "DROP FOREIGN KEY"
	}

	return "ALTER TABLE " + d.QuoteIdent(b.table.Name) + " " + verb + " " + d.QuoteIdent(b.name), nil, nil
}
