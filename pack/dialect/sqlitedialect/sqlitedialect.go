// Package sqlitedialect implements dialect.Dialect and dialect.DDL for
// SQLite.
package sqlitedialect

import (
	"strconv"
	"strings"

	"github.com/asimmons91/trails/pack/dialect"
)

// SQLite implements dialect.Dialect. Error classification lives in
// pack/driver/sqlite, not here - see driver.ErrorDecoder.
type SQLite struct{}

var _ dialect.Dialect = SQLite{}

// New returns a SQLite dialect.
func New() SQLite { return SQLite{} }

// Name returns "sqlite".
func (SQLite) Name() string { return "sqlite" }

// QuoteIdent double-quotes name, doubling any embedded double quote.
func (SQLite) QuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// Placeholder returns "?" + n; SQLite supports numbered placeholders.
func (SQLite) Placeholder(n int) string { return "?" + strconv.Itoa(n) }

// SupportsILike is false; SQLite's LIKE is case-insensitive for ASCII by
// default but has no distinct ILIKE operator.
func (SQLite) SupportsILike() bool { return false }

// SupportsRowLocking is false; SQLite has no row-level locking clauses.
func (SQLite) SupportsRowLocking() bool { return false }

// SupportsReturning is true; SQLite supports INSERT/UPDATE ... RETURNING.
func (SQLite) SupportsReturning() bool { return true }

// SupportsOnConflict is true; SQLite supports INSERT ... ON CONFLICT.
func (SQLite) SupportsOnConflict() bool { return true }

// SupportsSavepoints is true; SQLite supports SAVEPOINT.
func (SQLite) SupportsSavepoints() bool { return true }
