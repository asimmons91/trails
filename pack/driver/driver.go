// Package driver defines the pluggable database-engine surface
// pack.Connect opens a *pack.DB through: a Driver opens the connection and
// supplies its dialect.Dialect, and an ErrorDecoder translates that
// engine's native errors into pack's portable Code* constants. Each engine
// has its own implementation package (driver/mysql, driver/postgres,
// driver/sqlite).
package driver

import (
	"database/sql"

	"github.com/asimmons91/trails/pack/dialect"
)

// Driver opens a connection to one database engine and supplies the
// dialect.Dialect pack should use with it.
type Driver interface {
	// Name identifies the driver, e.g. in error messages.
	Name() string
	// Open opens a *sql.DB for dsn.
	Open(dsn string) (*sql.DB, error)
	// Dialect returns the dialect.Dialect this driver pairs with.
	Dialect() dialect.Dialect
}

// Code* are engine-agnostic error classification codes an
// ErrorDecoder.Classify returns, consumed by pack to construct the
// matching Err*Violation/ErrSerializationFailure type (see pack/classify.go).
const (
	CodeUnique               = "ERR_UNIQUE"
	CodeForeignKey           = "ERR_FOREIGN_KEY"
	CodeNotNull              = "ERR_NOT_NULL"
	CodeCheck                = "ERR_CHECK"
	CodeSerializationFailure = "ERR_SERIALIZATION_FAILURE"
)

// ErrorDecoder maps a raw driver error to one of the Code* constants.
type ErrorDecoder interface {
	// Classify returns the Code* matching err, or ok=false if it isn't a
	// classifiable database error.
	Classify(err error) (code string, ok bool)
}

// DetailedErrorDecoder extends ErrorDecoder with the offending
// constraint/table/column name, when the underlying driver exposes it.
// It's an extension point — no current driver implementation uses it.
type DetailedErrorDecoder interface {
	ErrorDecoder

	// Detail extracts the constraint/table/column implicated by err, when
	// available; any of the three may be empty.
	Detail(err error) (constraint, table, column string)
}
