package sqlbuild

import "github.com/asimmons91/trails/pack/dialect"

// DropTableBuilder renders a DROP TABLE statement. Build one with
// DropTable.
type DropTableBuilder struct{ table Table }

// DropTable starts a DropTableBuilder for t.
func DropTable(t Table) *DropTableBuilder { return &DropTableBuilder{table: t} }

// Render renders b as DROP TABLE SQL for dialect d.
func (b *DropTableBuilder) Render(d dialect.Dialect) (string, []any, error) {
	return "DROP TABLE " + d.QuoteIdent(b.table.Name), nil, nil
}

// RenameTableBuilder renders an ALTER TABLE ... RENAME TO statement. Build
// one with RenameTable.
type RenameTableBuilder struct {
	table   Table
	newName string
}

// RenameTable starts a RenameTableBuilder renaming t to newName.
func RenameTable(t Table, newName string) *RenameTableBuilder {
	return &RenameTableBuilder{table: t, newName: newName}
}

// Render renders b as ALTER TABLE ... RENAME TO SQL for dialect d.
func (b *RenameTableBuilder) Render(d dialect.Dialect) (string, []any, error) {
	return "ALTER TABLE " + d.QuoteIdent(b.table.Name) + " RENAME TO " + d.QuoteIdent(b.newName), nil, nil
}
