// Package postgres implements driver.Driver and driver.ErrorDecoder for
// Postgres, via github.com/jackc/pgx.
package postgres

import (
	"database/sql"
	"errors"

	"github.com/asimmons91/trails/pack/dialect"
	"github.com/asimmons91/trails/pack/dialect/pgdialect"
	"github.com/asimmons91/trails/pack/driver"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// Driver implements driver.Driver and driver.ErrorDecoder for Postgres.
type Driver struct{}

var _ driver.Driver = Driver{}

// New returns a Postgres Driver.
func New() Driver { return Driver{} }

// Name returns "pgx", the registered database/sql driver name.
func (Driver) Name() string { return "pgx" }

// Open opens dsn via the pgx stdlib driver.
func (Driver) Open(dsn string) (*sql.DB, error) {
	return sql.Open("pgx", dsn)
}

// Dialect returns a pgdialect.Postgres.
func (Driver) Dialect() dialect.Dialect { return pgdialect.New() }

var _ driver.ErrorDecoder = Driver{}

// Classify maps a *pgconn.PgError's SQLSTATE code to a driver.Code*
// constant, per the table below; any other error, or a non-Postgres
// error, returns ok=false.
func (Driver) Classify(err error) (string, bool) {
	var pe *pgconn.PgError
	if !errors.As(err, &pe) {
		return "", false
	}
	switch pe.SQLState() {
	case "23505": // unique_violation
		return driver.CodeUnique, true
	case "23503": // foreign_key_violation
		return driver.CodeForeignKey, true
	case "23502": // not_null_violation
		return driver.CodeNotNull, true
	case "23514": // check_violation
		return driver.CodeCheck, true
	case "40001": // serialization_failure
		return driver.CodeSerializationFailure, true
	case "40P01": // deadlock_detected
		return driver.CodeSerializationFailure, true
	default:
		return "", false
	}
}
