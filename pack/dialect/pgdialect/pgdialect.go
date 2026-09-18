// Package pgdialect implements dialect.Dialect and dialect.DDL for
// Postgres.
package pgdialect

import (
	"strconv"
	"strings"

	"github.com/asimmons91/trails/pack/dialect"
)

// Postgres implements dialect.Dialect. Error classification lives in
// pack/driver/postgres, not here - see driver.ErrorDecoder.
type Postgres struct{}

var _ dialect.Dialect = Postgres{}

// New returns a Postgres dialect.
func New() Postgres { return Postgres{} }

// Name returns "postgres".
func (Postgres) Name() string { return "postgres" }

// QuoteIdent double-quotes name, doubling any embedded double quote.
func (Postgres) QuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// Placeholder returns "$" + n; Postgres uses numbered placeholders.
func (Postgres) Placeholder(n int) string {
	return "$" + strconv.Itoa(n)
}

// SupportsILike is true; Postgres has a native ILIKE operator.
func (Postgres) SupportsILike() bool { return true }

// SupportsRowLocking is true; Postgres supports FOR UPDATE/FOR SHARE.
func (Postgres) SupportsRowLocking() bool { return true }

// SupportsReturning is true; Postgres supports INSERT/UPDATE ... RETURNING.
func (Postgres) SupportsReturning() bool { return true }

// SupportsOnConflict is true; Postgres supports INSERT ... ON CONFLICT.
func (Postgres) SupportsOnConflict() bool { return true }

// SupportsSavepoints is true; Postgres supports SAVEPOINT.
func (Postgres) SupportsSavepoints() bool { return true }
