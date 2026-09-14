package pack

import (
	"context"
	"database/sql/driver"
	"strings"
	"testing"
	"time"

	"github.com/asimmons91/trails/pack/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeMySQLDialect mimics the traits a real MySQL dialect will have
// (backtick quoting, plain "?" placeholders, no RETURNING, no ON CONFLICT)
// without depending on a real mysqldialect package, which doesn't exist
// yet. It exists purely to prove Create/CreateAll's RETURNING-fallback
// branches work before any MySQL-specific code lands.
type fakeMySQLDialect struct{}

func (fakeMySQLDialect) Name() string { return "mysql-shaped-fake" }
func (fakeMySQLDialect) QuoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}
func (fakeMySQLDialect) Placeholder(int) string   { return "?" }
func (fakeMySQLDialect) SupportsILike() bool      { return false }
func (fakeMySQLDialect) SupportsRowLocking() bool { return true }
func (fakeMySQLDialect) SupportsReturning() bool  { return false }
func (fakeMySQLDialect) SupportsOnConflict() bool { return false }

func newMySQLShapedTestDB() (*DB, *testdb.FakeDB) {
	fake := testdb.New()
	db := Open(fake.Open(), fakeMySQLDialect{})
	return db, fake
}

func TestCreate_NoReturningDialect_AutoIncrementOnly_UsesLastInsertId(t *testing.T) {
	db, fake := newMySQLShapedTestDB()
	fake.Enqueue(testdb.Result{LastInsertID: 5})

	row := &testWidget{Count: 7}
	err := Create(context.Background(), db, row)
	require.NoError(t, err)

	executed := fake.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t, "INSERT INTO `widgets` (`count`) VALUES (?)", executed[0].SQL)
	assert.Equal(t, []any{uint64(7)}, executed[0].Args)
	assert.EqualValues(t, 5, row.ID)
}

func TestCreate_NoReturningDialect_HasDefaultColumn_IssuesFollowUpSelect(t *testing.T) {
	db, fake := newMySQLShapedTestDB()
	fake.Enqueue(testdb.Result{LastInsertID: 2}) // the INSERT
	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	fake.Enqueue(testdb.Result{ // the follow-up SELECT for created_at
		Columns: []string{"created_at"},
		Rows:    [][]driver.Value{testdb.Row(createdAt)},
	})

	row := &testAccount{Email: "grace@example.com"}
	err := Create(context.Background(), db, row)
	require.NoError(t, err)

	executed := fake.Executed()
	require.Len(t, executed, 2)
	assert.Equal(t, "INSERT INTO `accounts` (`email`, `nickname`) VALUES (?, ?)", executed[0].SQL)
	assert.Equal(t, []any{"grace@example.com", nil}, executed[0].Args)
	assert.Equal(t, "SELECT `created_at` FROM `accounts` WHERE `id` = ?", executed[1].SQL)
	assert.Equal(t, []any{int64(2)}, executed[1].Args)

	assert.EqualValues(t, 2, row.ID)
	assert.True(t, row.CreatedAt.Equal(createdAt))
}

func TestCreateAll_NoReturningDialect_BackfillsContiguousAutoIncrementIds(t *testing.T) {
	db, fake := newMySQLShapedTestDB()
	fake.Enqueue(testdb.Result{LastInsertID: 10})

	rows := []*testWidget{{Count: 1}, {Count: 2}, {Count: 3}}
	err := CreateAll(context.Background(), db, rows)
	require.NoError(t, err)

	executed := fake.Executed()
	require.Len(t, executed, 1)
	assert.Equal(t, "INSERT INTO `widgets` (`count`) VALUES (?), (?), (?)", executed[0].SQL)

	assert.EqualValues(t, 10, rows[0].ID)
	assert.EqualValues(t, 11, rows[1].ID)
	assert.EqualValues(t, 12, rows[2].ID)
}
