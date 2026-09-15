package sqlbuild

import "github.com/asimmons91/trails/pack/dialect"

type DropTableBuilder struct{ table Table }

func DropTable(t Table) *DropTableBuilder { return &DropTableBuilder{table: t} }

func (b *DropTableBuilder) Render(d dialect.Dialect) (string, []any, error) {
	return "DROP TABLE " + d.QuoteIdent(b.table.Name), nil, nil
}

type RenameTableBuilder struct {
	table   Table
	newName string
}

func RenameTable(t Table, newName string) *RenameTableBuilder {
	return &RenameTableBuilder{table: t, newName: newName}
}

func (b *RenameTableBuilder) Render(d dialect.Dialect) (string, []any, error) {
	return "ALTER TABLE " + d.QuoteIdent(b.table.Name) + " RENAME TO " + d.QuoteIdent(b.newName), nil, nil
}
