package mysql

import (
	"database/sql"
	"errors"

	"github.com/asimmons91/trails/pack/dialect"
	"github.com/asimmons91/trails/pack/dialect/mysqldialect"
	"github.com/asimmons91/trails/pack/driver"
	sqlmysql "github.com/go-sql-driver/mysql"
)

type Driver struct{}

var _ driver.Driver = Driver{}

func New() Driver { return Driver{} }

func (Driver) Name() string { return "mysql" }

func (Driver) Open(dsn string) (*sql.DB, error) {
	return sql.Open("mysql", dsn)
}

func (Driver) Dialect() dialect.Dialect { return mysqldialect.New() }

var _ driver.ErrorDecoder = Driver{}

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
	default:
		return "", false
	}
}
