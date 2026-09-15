package sqlbuild

import (
	"strings"

	"github.com/asimmons91/trails/pack/dialect"
)

type CreateConstraintBuilder struct {
	table     Table
	name      string
	localCols []string
	refTable  string
	refCols   []string
}

func CreateForeignKeyConstraint(t Table, name string, localCols []string, refTable string, refCols []string) *CreateConstraintBuilder {
	return &CreateConstraintBuilder{
		table:     t,
		name:      name,
		localCols: append([]string(nil), localCols...),
		refTable:  refTable,
		refCols:   append([]string(nil), refCols...),
	}
}

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

type DropConstraintBuilder struct {
	table Table
	name  string
}

func DropConstraint(t Table, name string) *DropConstraintBuilder {
	return &DropConstraintBuilder{table: t, name: name}
}

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
