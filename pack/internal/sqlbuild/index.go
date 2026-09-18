package sqlbuild

import (
	"strings"

	"github.com/asimmons91/trails/pack/dialect"
)

// CreateIndexBuilder builds a CREATE INDEX statement. Build one with
// CreateIndex.
type CreateIndexBuilder struct {
	table   Table
	name    string
	columns []string
	unique  bool
}

// CreateIndex starts a CreateIndexBuilder creating an index named name on
// t's columns.
func CreateIndex(t Table, name string, columns ...string) *CreateIndexBuilder {
	return &CreateIndexBuilder{table: t, name: name, columns: append([]string(nil), columns...)}
}

func (b *CreateIndexBuilder) clone() *CreateIndexBuilder {
	nb := *b
	nb.columns = append([]string(nil), b.columns...)
	return &nb
}

// Unique makes the index a UNIQUE INDEX.
func (b *CreateIndexBuilder) Unique() *CreateIndexBuilder {
	nb := b.clone()
	nb.unique = true
	return nb
}

// Render renders b as CREATE [UNIQUE] INDEX SQL for dialect d.
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

// DropIndexBuilder builds a statement dropping a named index. Build one
// with DropIndex.
type DropIndexBuilder struct {
	table Table
	index string
}

// DropIndex starts a DropIndexBuilder dropping index from t.
func DropIndex(t Table, index string) *DropIndexBuilder {
	return &DropIndexBuilder{table: t, index: index}
}

// Render renders b via dialect.DDL.DropIndexSQL. It errors with
// ErrDDLUnsupportedByDialect if d doesn't implement dialect.DDL.
func (b *DropIndexBuilder) Render(d dialect.Dialect) (string, []any, error) {
	ddl, ok := d.(dialect.DDL)
	if !ok {
		return "", nil, &ErrDDLUnsupportedByDialect{Dialect: d.Name()}
	}

	return ddl.DropIndexSQL(b.table.Name, b.index), nil, nil
}

// RenameIndexBuilder builds a statement renaming an index. Build one with
// RenameIndex.
type RenameIndexBuilder struct {
	table            Table
	oldName, newName string
}

// RenameIndex starts a RenameIndexBuilder renaming t's oldName index to
// newName.
func RenameIndex(t Table, oldName, newName string) *RenameIndexBuilder {
	return &RenameIndexBuilder{table: t, oldName: oldName, newName: newName}
}

// Render renders b via dialect.DDL.RenameIndexSQL, which errors on a
// dialect with no way to rename an index in place (SQLite). It errors
// with ErrDDLUnsupportedByDialect if d doesn't implement dialect.DDL at
// all.
func (b *RenameIndexBuilder) Render(d dialect.Dialect) (string, []any, error) {
	ddl, ok := d.(dialect.DDL)
	if !ok {
		return "", nil, &ErrDDLUnsupportedByDialect{Dialect: d.Name()}
	}

	sqlText, err := ddl.RenameIndexSQL(b.table.Name, b.oldName, b.newName)
	return sqlText, nil, err
}

// quoteIdentList quotes and comma-joins names for use in a column list.
func quoteIdentList(d dialect.Dialect, names []string) string {
	parts := make([]string, len(names))
	for i, n := range names {
		parts[i] = d.QuoteIdent(n)
	}
	return strings.Join(parts, ", ")
}
