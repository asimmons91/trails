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

const (
	CodeUnique               = "ERR_UNIQUE"
	CodeForeignKey           = "ERR_FOREIGN_KEY"
	CodeNotNull              = "ERR_NOT_NULL"
	CodeCheck                = "ERR_CHECK"
	CodeSerializationFailure = "ERR_SERIALIZATION_FAILURE"
)

type ErrorDecoder interface {
	Classify(err error) (code string, ok bool)
}

type DetailedErrorDecoder interface {
	ErrorDecoder

	Detail(err error) (constraint, table, column string)
}
