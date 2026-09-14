package sqlite

import (
	"database/sql"

	"github.com/asimmons91/trails/pack/dialect"
	"github.com/asimmons91/trails/pack/dialect/sqlitedialect"
	"github.com/asimmons91/trails/pack/driver"
	_ "modernc.org/sqlite"
)

type Driver struct{}

var _ driver.Driver = Driver{}

func New() Driver { return Driver{} }

func (Driver) Name() string { return "sqlite" }

func (Driver) Open(dsn string) (*sql.DB, error) {
	return sql.Open("sqlite", dsn)
}

func (Driver) Dialect() dialect.Dialect { return sqlitedialect.New() }
