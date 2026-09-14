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

type Driver struct{}

var _ driver.Driver = Driver{}

func New() Driver { return Driver{} }

func (Driver) Name() string { return "pgx" }

func (Driver) Open(dsn string) (*sql.DB, error) {
	return sql.Open("pgx", dsn)
}

func (Driver) Dialect() dialect.Dialect { return pgdialect.New() }

var _ driver.ErrorDecoder = Driver{}

func (Driver) Classify(err error) (string, bool) {
	var pe *pgconn.PgError
	if !errors.As(err, &pe) {
		return "", false
	}
	switch pe.SQLState() {
	case "23505":
		return driver.CodeUnique, true
	case "23503":
		return driver.CodeForeignKey, true
	case "23502":
		return driver.CodeNotNull, true
	case "23514":
		return driver.CodeCheck, true
	default:
		return "", false
	}
}
