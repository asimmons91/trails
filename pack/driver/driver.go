package driver

import (
	"database/sql"

	"github.com/asimmons91/trails/pack/dialect"
)

type Driver interface {
	Name() string
	Open(dsn string) (*sql.DB, error)
	Dialect() dialect.Dialect
}
