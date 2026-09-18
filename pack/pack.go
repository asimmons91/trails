// Package pack is a type-safe ORM over database/sql. Query[T]/Of build and
// run reads; Create, Update, Save, Delete, and ByID handle single-row
// writes; CreateAll batches inserts. A mapped struct embeds Model[ID] (or
// otherwise implements Entity[ID]) and describes its columns/relations via
// `db:"..."` struct tags, parsed by pack/internal/schema. Open (or Connect,
// which also opens the connection) wraps a *sql.DB with a pack/dialect
// implementation to produce the *DB every other function in this package
// operates on.
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

// Option configures a DB created by Open or Connect.
type Option func(o *dbOptions)

// DB is a connection (or, inside Tx/BeginTx, a transaction) that every
// query/write function in this package takes as its first non-context
// argument. Construct one with Open or Connect.
type DB struct {
	conn         dbConn
	beginner     txBeginner
	dialect      dialect.Dialect
	opts         *dbOptions
	scope        txScope       // non-nil only for a *DB returned by BeginTx; nil for a root DB
	savepointSeq *atomic.Int64 // shared across an entire Tx/BeginTx tree; nil outside a transaction
}

// Open wraps an already-opened db so pack can query and write through it,
// generating SQL for the given dialect. Use Connect instead to open the
// *sql.DB and wrap it in one call, which also wires up d's error
// classification automatically.
func Open(db *sql.DB, d dialect.Dialect, opts ...Option) *DB {
	o := &dbOptions{errDecoder: noopErrorDecoder{}}
	for _, opt := range opts {
		opt(o)
	}
	return &DB{conn: db, beginner: db, dialect: d, opts: o}
}
