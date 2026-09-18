// Package mysql implements driver.Driver and driver.ErrorDecoder for
// MySQL, via github.com/go-sql-driver/mysql.
package mysql

import (
	"database/sql"
	"errors"

	"github.com/asimmons91/trails/pack/dialect"
	"github.com/asimmons91/trails/pack/dialect/mysqldialect"
	"github.com/asimmons91/trails/pack/driver"
	sqlmysql "github.com/go-sql-driver/mysql"
)

// Driver implements driver.Driver and driver.ErrorDecoder for MySQL.
type Driver struct{}

var _ driver.Driver = Driver{}

// New returns a MySQL Driver.
func New() Driver { return Driver{} }

// Name returns "mysql".
func (Driver) Name() string { return "mysql" }

// Open opens dsn via the go-sql-driver/mysql driver.
func (Driver) Open(dsn string) (*sql.DB, error) {
	return sql.Open("mysql", dsn)
}

// Dialect returns a mysqldialect.MySQL.
func (Driver) Dialect() dialect.Dialect { return mysqldialect.New() }

var _ driver.ErrorDecoder = Driver{}

// Classify maps a *mysql.MySQLError's error number to a driver.Code*
// constant, per the table below; any other error, or a non-MySQL error,
// returns ok=false.
func (Driver) Classify(err error) (string, bool) {
	var me *sqlmysql.MySQLError
	if !errors.As(err, &me) {
		return "", false
	}

	switch me.Number {
	case 1062: // ER_DUP_ENTRY
		return driver.CodeUnique, true
	case 1452: // ER_NO_REFERENCED_ROW_2
		return driver.CodeForeignKey, true
	case 1048: // ER_BAD_NULL_ERROR
		return driver.CodeNotNull, true
	case 3819: // ER_CHECK_CONSTRAINT_VIOLATED (MySQL 8.0.16+)
		return driver.CodeCheck, true
	case 1213: // ER_LOCK_DEADLOCK
		return driver.CodeSerializationFailure, true
	case 1205: // ER_LOCK_WAIT_TIMEOUT
		return driver.CodeSerializationFailure, true
	default:
		return "", false
	}
}
