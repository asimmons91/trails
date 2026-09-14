package pack

import (
	"context"
	"database/sql"
	"time"

	"github.com/asimmons91/trails/pack/dialect"
)

type BeforeInserter interface {
	BeforeInsert(ctx context.Context) error
}

type AfterInserter interface {
	AfterInsert(ctx context.Context) error
}

type BeforeUpdater interface {
	BeforeUpdate(ctx context.Context) error
}

type AfterUpdater interface {
	AfterUpdate(ctx context.Context) error
}

type BeforeDeleter interface {
	BeforeDelete(ctx context.Context) error
}

type AfterDeleter interface {
	AfterDelete(ctx context.Context) error
}

type AfterScanner interface {
	AfterScan(ctx context.Context) error
}

type QueryEvent struct {
	Operation    string
	Model        string
	SQL          string
	Args         []any
	Start        time.Time
	Duration     time.Duration
	RowsAffected int64
	Err          error
}

type QueryHook interface {
	BeforeQuery(ctx context.Context, ev *QueryEvent) context.Context
	AfterQuery(ctx context.Context, ev *QueryEvent)
}

func WithQueryHook(h QueryHook) Option {
	return func(o *dbOptions) {
		o.queryHooks = append(o.queryHooks, h)
	}
}

func WithQueryHookArgs() Option {
	return func(o *dbOptions) {
		o.exposeQueryArgs = true
	}
}

func (db *DB) beforeQuery(ctx context.Context, ev *QueryEvent) context.Context {
	ev.Start = time.Now()
	for _, h := range db.opts.queryHooks {
		ctx = h.BeforeQuery(ctx, ev)
	}

	return ctx
}

func (db *DB) afterQuery(ctx context.Context, ev *QueryEvent) {
	ev.Duration = time.Since(ev.Start)
	for _, h := range db.opts.queryHooks {
		h.AfterQuery(ctx, ev)
	}
}

func (db *DB) queryContext(ctx context.Context, op, model, sqlText string, args []any) (*sql.Rows, error) {
	ev := &QueryEvent{Operation: op, Model: model, SQL: sqlText}
	if db.opts.exposeQueryArgs {
		ev.Args = args
	}

	ctx = db.beforeQuery(ctx, ev)
	rows, err := db.conn.QueryContext(ctx, sqlText, args...)
	err = classifyError(db, op, model, err)
	ev.Err = err
	db.afterQuery(ctx, ev)

	return rows, err
}

func (db *DB) execContext(ctx context.Context, op, model, sqlText string, args []any) (sql.Result, error) {
	ev := &QueryEvent{Operation: op, Model: model, SQL: sqlText}
	if db.opts.exposeQueryArgs {
		ev.Args = args
	}
	ctx = db.beforeQuery(ctx, ev)
	res, err := db.conn.ExecContext(ctx, sqlText, args...)
	if err == nil {
		if n, rerr := res.RowsAffected(); rerr == nil {
			ev.RowsAffected = n
		}
	}
	err = classifyError(db, op, model, err)
	ev.Err = err
	db.afterQuery(ctx, ev)
	return res, err
}

func (db *DB) Dialect() dialect.Dialect { return db.dialect }

func (db *DB) ExecContext(ctx context.Context, op, model, sqlText string, args []any) (sql.Result, error) {
	return db.execContext(ctx, op, model, sqlText, args)
}

func (db *DB) QueryContext(ctx context.Context, op, model, sqlText string, args []any) (*sql.Rows, error) {
	return db.queryContext(ctx, op, model, sqlText, args)
}
