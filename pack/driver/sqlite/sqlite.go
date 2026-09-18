// Package sqlite implements driver.Driver and driver.ErrorDecoder for
// SQLite, via modernc.org/sqlite (a CGo-free driver).
package sqlite

import (
	"database/sql"
	"errors"

	"github.com/asimmons91/trails/pack/dialect"
	"github.com/asimmons91/trails/pack/dialect/sqlitedialect"
	"github.com/asimmons91/trails/pack/driver"
	sqlitedriver "modernc.org/sqlite"
)

// Driver implements driver.Driver and driver.ErrorDecoder for SQLite.
type Driver struct{}

var _ driver.Driver = Driver{}

// New returns a SQLite Driver.
func New() Driver { return Driver{} }

// Name returns "sqlite".
func (Driver) Name() string { return "sqlite" }

// Open opens dsn via the modernc.org/sqlite driver.
func (Driver) Open(dsn string) (*sql.DB, error) {
	return sql.Open("sqlite", dsn)
}

// Dialect returns a sqlitedialect.SQLite.
func (Driver) Dialect() dialect.Dialect { return sqlitedialect.New() }

var _ driver.ErrorDecoder = Driver{}

// Classify maps a *sqlite.Error's extended result code to a driver.Code*
// constant, per the table below; any other error, or a non-SQLite error,
// returns ok=false.
func (Driver) Classify(err error) (string, bool) {
	var se *sqlitedriver.Error
	if !errors.As(err, &se) {
		return "", false
	}
	switch se.Code() {
	case 2067, 1555: // SQLITE_CONSTRAINT_UNIQUE, SQLITE_CONSTRAINT_PRIMARYKEY
		return driver.CodeUnique, true
	case 787: // SQLITE_CONSTRAINT_FOREIGNKEY
		return driver.CodeForeignKey, true
	case 1299: // SQLITE_CONSTRAINT_NOTNULL
		return driver.CodeNotNull, true
	case 275: // SQLITE_CONSTRAINT_CHECK
		return driver.CodeCheck, true
	case 5, 261, 517, 773: // SQLITE_BUSY and its extended (RECOVERY/SNAPSHOT/TIMEOUT) variants
		return driver.CodeSerializationFailure, true
	case 6, 262, 518: // SQLITE_LOCKED and its extended (SHAREDCACHE/VTAB) variants
		return driver.CodeSerializationFailure, true
	default:
		return "", false
	}
}
