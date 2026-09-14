package postgres

import (
	"database/sql"

	"github.com/asimmons91/trails/pack/dialect"
	"github.com/asimmons91/trails/pack/dialect/pgdialect"
	"github.com/asimmons91/trails/pack/driver"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type Driver struct{}

var _ driver.Driver = Driver{}

func New() Driver { return Driver{} }

func (Driver) Name() string { return "pgx" }

func (Driver) Open(dsn string) (*sql.DB, error) {
	return sql.Open("pgx", dsn)
}

func (Driver) Dialect() dialect.Dialect { return pgdialect.New() }
