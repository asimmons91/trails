package pack

import (
	"context"
	"database/sql"

	"github.com/asimmons91/trails/pack/dialect"
)

type dbConn interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type txBeginner interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

type dbOptions struct {
	queryHooks      []QueryHook
	exposeQueryArgs bool
	errDecoder      dialect.ErrorDecoder
}

type Option func(o *dbOptions)

type DB struct {
	conn     dbConn
	beginner txBeginner
	dialect  dialect.Dialect
	opts     *dbOptions
	tx       *sql.Tx
}

func Open(db *sql.DB, d dialect.Dialect, opts ...Option) *DB {
	o := &dbOptions{errDecoder: noopErrorDecoder{}}
	if ed, ok := d.(dialect.ErrorDecoder); ok {
		o.errDecoder = ed
	}
	for _, opt := range opts {
		opt(o)
	}
	return &DB{conn: db, beginner: db, dialect: d, opts: o}
}
