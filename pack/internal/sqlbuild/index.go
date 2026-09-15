package sqlbuild

import (
	"strings"

	"github.com/asimmons91/trails/pack/dialect"
)

type CreateIndexBuilder struct {
	table   Table
	name    string
	columns []string
	unique  bool
}

func CreateIndex(t Table, name string, columns ...string) *CreateIndexBuilder {
	return &CreateIndexBuilder{table: t, name: name, columns: append([]string(nil), columns...)}
}

func (b *CreateIndexBuilder) clone() *CreateIndexBuilder {
	nb := *b
	nb.columns = append([]string(nil), b.columns...)
	return &nb
}

func (b *CreateIndexBuilder) Unique() *CreateIndexBuilder {
	nb := b.clone()
	nb.unique = true
	return nb
}

func (b *CreateIndexBuilder) Render(d dialect.Dialect) (string, []any, error) {
	var sb strings.Builder
	sb.WriteString("CREATE ")
	if b.unique {
		sb.WriteString("UNIQUE ")
	}
	sb.WriteString("INDEX ")
	sb.WriteString(d.QuoteIdent(b.name))
	sb.WriteString(" ON ")
	sb.WriteString(d.QuoteIdent(b.table.Name))
	sb.WriteString(" (")
	sb.WriteString(quoteIdentList(d, b.columns))
	sb.WriteString(")")

	return sb.String(), nil, nil
}

type DropIndexBuilder struct {
	table Table
	index string
}

func DropIndex(t Table, index string) *DropIndexBuilder {
	return &DropIndexBuilder{table: t, index: index}
}

func (b *DropIndexBuilder) Render(d dialect.Dialect) (string, []any, error) {
	ddl, ok := d.(dialect.DDL)
	if !ok {
		return "", nil, &ErrDDLUnsupportedByDialect{Dialect: d.Name()}
	}

	return ddl.DropIndexSQL(b.table.Name, b.index), nil, nil
}

type RenameIndexBuilder struct {
	table            Table
	oldName, newName string
}

func RenameIndex(t Table, oldName, newName string) *RenameIndexBuilder {
	return &RenameIndexBuilder{table: t, oldName: oldName, newName: newName}
}

func (b *RenameIndexBuilder) Render(d dialect.Dialect) (string, []any, error) {
	ddl, ok := d.(dialect.DDL)
	if !ok {
		return "", nil, &ErrDDLUnsupportedByDialect{Dialect: d.Name()}
	}

	sqlText, err := ddl.RenameIndexSQL(b.table.Name, b.oldName, b.newName)
	return sqlText, nil, err
}

func quoteIdentList(d dialect.Dialect, names []string) string {
	parts := make([]string, len(names))
	for i, n := range names {
		parts[i] = d.QuoteIdent(n)
	}
	return strings.Join(parts, ", ")
}
