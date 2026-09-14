package pack

import (
	"context"
	"database/sql"
	"sync/atomic"

	"github.com/asimmons91/trails/pack/dialect"
	"github.com/asimmons91/trails/pack/driver"
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
	errDecoder      driver.ErrorDecoder
}

type Option func(o *dbOptions)

type DB struct {
	conn         dbConn
	beginner     txBeginner
	dialect      dialect.Dialect
	opts         *dbOptions
	scope        txScope       // nil outside a transaction
	savepointSeq *atomic.Int64 // shared across an entire tx tree; nil outside a transaction
}

func Open(db *sql.DB, d dialect.Dialect, opts ...Option) *DB {
	o := &dbOptions{errDecoder: noopErrorDecoder{}}
	for _, opt := range opts {
		opt(o)
	}
	return &DB{conn: db, beginner: db, dialect: d, opts: o}
}
