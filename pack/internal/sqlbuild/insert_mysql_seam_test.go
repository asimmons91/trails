package sqlbuild

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// mysqlShapedFake mimics the traits a real MySQL dialect will have
// (backtick quoting, plain "?" placeholders, no RETURNING, no ON CONFLICT)
// without depending on a real mysqldialect package, which doesn't exist
// yet. It exists purely to prove sqlbuild's new capability branches render
// correctly before any MySQL-specific code lands.
type mysqlShapedFake struct{}

func (mysqlShapedFake) Name() string { return "mysql-shaped-fake" }
func (mysqlShapedFake) QuoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}
func (mysqlShapedFake) Placeholder(int) string   { return "?" }
func (mysqlShapedFake) SupportsILike() bool      { return false }
func (mysqlShapedFake) SupportsRowLocking() bool { return true }
func (mysqlShapedFake) SupportsReturning() bool  { return false }
func (mysqlShapedFake) SupportsOnConflict() bool { return false }

var mysqlFake = mysqlShapedFake{}

func TestInsert_OnConflictDoNothing_MySQLStyle_RendersInsertIgnore(t *testing.T) {
	sql, args, err := Insert(Table{Name: "users"}).
		Values(Set(Col("email"), "a@b.com")).
		OnConflictDoNothing(Col("email")).
		Render(mysqlFake)
	require.NoError(t, err)
	require.Equal(t, "INSERT IGNORE INTO `users` (`email`) VALUES (?)", sql)
	require.Equal(t, []any{"a@b.com"}, args)
}

func TestInsert_OnConflictDoUpdate_MySQLStyle_RendersOnDuplicateKeyUpdate(t *testing.T) {
	sql, args, err := Insert(Table{Name: "users"}).
		Values(Set(Col("email"), "a@b.com"), Set(Col("name"), "Bob")).
		OnConflictDoUpdate(
			[]Column{Col("email")},
			Set(Col("name"), "Bobby"),
		).
		Render(mysqlFake)
	require.NoError(t, err)
	require.Equal(t, "INSERT INTO `users` (`email`, `name`) VALUES (?, ?) ON DUPLICATE KEY UPDATE `name` = ?", sql)
	require.Equal(t, []any{"a@b.com", "Bob", "Bobby"}, args)
}

func TestInsert_Returning_UnsupportedByDialect_ReturnsError(t *testing.T) {
	_, _, err := Insert(Table{Name: "users"}).
		Values(Set(Col("email"), "a@b.com")).
		Returning(Col("id")).
		Render(mysqlFake)
	require.Error(t, err)
	require.IsType(t, &ErrReturningUnsupportedByDialect{}, err)
}

func TestInsert_MultiRow_MySQLStyle_PlaceholdersAreUnnumbered(t *testing.T) {
	sql, args, err := Insert(Table{Name: "users"}).
		Values(Set(Col("email"), "a@b.com")).
		Values(Set(Col("email"), "c@d.com")).
		Render(mysqlFake)
	require.NoError(t, err)
	require.Equal(t, "INSERT INTO `users` (`email`) VALUES (?), (?)", sql)
	require.Equal(t, []any{"a@b.com", "c@d.com"}, args)
}
