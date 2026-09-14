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

func (Driver) Dialect() dialect.Dialect { return mysqlDialect{mysqldialect.New()} }

type mysqlDialect struct{ mysqldialect.MySQL }

var _ dialect.ErrorDecoder = mysqlDialect{}

func (mysqlDialect) Classify(err error) (string, bool) {
	var me *sqlmysql.MySQLError
	if !errors.As(err, &me) {
		return "", false
	}

	switch me.Number {
	case 1062: // ER_DUP_ENTRY
		return dialect.CodeUnique, true
	case 1452: // ER_NO_REFERENCED_ROW_2
		return dialect.CodeForeignKey, true
	case 1048: // ER_BAD_NULL_ERROR
		return dialect.CodeNotNull, true
	case 3819: // ER_CHECK_CONSTRAINT_VIOLATED (MySQL 8.0.16+)
		return dialect.CodeCheck, true
	default:
		return "", false
	}
}
