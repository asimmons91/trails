package sqlbuild

import (
	"reflect"
	"strings"

	"github.com/asimmons91/trails/pack/dialect"
)

type ddlColumn struct {
	name   string
	goType reflect.Type
	spec   dialect.ColumnSpec
}

type CreateTableBuilder struct {
	table       Table
	ifNotExists bool
	columns     []ddlColumn
}

func CreateTable(t Table) *CreateTableBuilder {
	return &CreateTableBuilder{table: t}
}

func (b *CreateTableBuilder) clone() *CreateTableBuilder {
	nb := *b
	nb.columns = append([]ddlColumn(nil), b.columns...)
	return &nb
}

func (b *CreateTableBuilder) IfNotExists() *CreateTableBuilder {
	nb := b.clone()
	nb.ifNotExists = true
	return nb
}

func (b *CreateTableBuilder) Column(name string, goType reflect.Type, spec dialect.ColumnSpec) *CreateTableBuilder {
	nb := b.clone()
	nb.columns = append(nb.columns, ddlColumn{name: name, goType: goType, spec: spec})
	return nb
}

func (b *CreateTableBuilder) Render(d dialect.Dialect) (string, []any, error) {
	ddl, ok := d.(dialect.DDL)
	if !ok {
		return "", nil, &ErrDDLUnsupportedByDialect{Dialect: d.Name()}
	}

	var sb strings.Builder
	sb.WriteString("CREATE TABLE ")
	if b.ifNotExists {
		sb.WriteString("IF NOT EXISTS ")
	}
	sb.WriteString(d.QuoteIdent(b.table.Name))
	sb.WriteString(" (")

	parts := make([]string, 0, len(b.columns)+1)
	var pkCols []string
	for _, c := range b.columns {
		def, err := ddl.ColumnDefinition(c.goType, c.spec)
		if err != nil {
			return "", nil, err
		}
		parts = append(parts, d.QuoteIdent(c.name)+" "+def)

		if c.spec.PrimaryKey && !ddl.InlinesPrimaryKey(c.spec) {
			pkCols = append(pkCols, c.name)
		}
	}

	if len(pkCols) > 0 {
		quoted := make([]string, len(pkCols))
		for i, c := range pkCols {
			quoted[i] = d.QuoteIdent(c)
		}
		parts = append(parts, "PRIMARY KEY ("+strings.Join(quoted, ", ")+")")
	}

	sb.WriteString(strings.Join(parts, ", "))
	sb.WriteString(")")

	return sb.String(), nil, nil
}
