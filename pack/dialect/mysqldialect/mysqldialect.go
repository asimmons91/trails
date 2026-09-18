// Package mysqldialect implements dialect.Dialect and dialect.DDL for
// MySQL.
package mysqldialect

import (
	"strings"

	"github.com/asimmons91/trails/pack/dialect"
)

// MySQL implements dialect.Dialect. Error classification lives in
// pack/driver/mysql, not here - see driver.ErrorDecoder.
type MySQL struct{}

var _ dialect.Dialect = MySQL{}

// New returns a MySQL dialect.
func New() MySQL { return MySQL{} }

// Name returns "mysql".
func (MySQL) Name() string { return "mysql" }

// QuoteIdent backtick-quotes name, doubling any embedded backtick.
func (MySQL) QuoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

// Placeholder returns "?"; MySQL placeholders are positional, not numbered.
func (MySQL) Placeholder(int) string { return "?" }

// SupportsILike is false; MySQL's LIKE is case-insensitive by default
// (collation-dependent) and has no distinct ILIKE operator.
func (MySQL) SupportsILike() bool { return false }

// SupportsRowLocking is true; MySQL supports FOR UPDATE/FOR SHARE.
func (MySQL) SupportsRowLocking() bool { return true }

// SupportsReturning is false; MySQL has no INSERT/UPDATE ... RETURNING.
func (MySQL) SupportsReturning() bool { return false }

// SupportsOnConflict is false; MySQL uses ON DUPLICATE KEY UPDATE instead
// of a standard ON CONFLICT clause, which this package doesn't generate.
func (MySQL) SupportsOnConflict() bool { return false }

// SupportsSavepoints is true; MySQL supports SAVEPOINT.
func (MySQL) SupportsSavepoints() bool { return true }
