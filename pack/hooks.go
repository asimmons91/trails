package pack

import (
	"context"
	"database/sql"
	"time"

	"github.com/asimmons91/trails/pack/dialect"
)

// BeforeInserter is implemented by a model that wants to run logic
// immediately before Create/CreateAll inserts it.
type BeforeInserter interface {
	BeforeInsert(ctx context.Context) error
}

// AfterInserter is implemented by a model that wants to run logic
// immediately after Create/CreateAll inserts it.
type AfterInserter interface {
	AfterInsert(ctx context.Context) error
}

// BeforeUpdater is implemented by a model that wants to run logic
// immediately before Update writes it.
type BeforeUpdater interface {
	BeforeUpdate(ctx context.Context) error
}

// AfterUpdater is implemented by a model that wants to run logic
// immediately after Update writes it.
type AfterUpdater interface {
	AfterUpdate(ctx context.Context) error
}

// BeforeDeleter is implemented by a model that wants to run logic
// immediately before Delete removes it.
type BeforeDeleter interface {
	BeforeDelete(ctx context.Context) error
}

// AfterDeleter is implemented by a model that wants to run logic
// immediately after Delete removes it.
type AfterDeleter interface {
	AfterDelete(ctx context.Context) error
}

// AfterScanner is implemented by a model that wants to run logic
// immediately after being scanned from a query result (Find, First, Rows,
// Raw).
type AfterScanner interface {
	AfterScan(ctx context.Context) error
}

// QueryEvent describes one query/exec pack has issued, passed to a
// QueryHook's BeforeQuery (partially filled in) and AfterQuery (complete).
type QueryEvent struct {
	Operation string
	Model     string
	SQL       string
	// Args holds the query's bound arguments, but only if WithQueryHookArgs
	// was set on the DB — otherwise it's always nil, since args may
	// contain sensitive values.
	Args         []any
	Start        time.Time
	Duration     time.Duration
	RowsAffected int64
	Err          error
}

// QueryHook brackets every query/exec a DB issues: BeforeQuery runs just
// before, and may return a modified context threaded through to
// AfterQuery, which runs just after with ev fully populated (including
// Err, if the query failed). Register one via WithQueryHook.
type QueryHook interface {
	BeforeQuery(ctx context.Context, ev *QueryEvent) context.Context
	AfterQuery(ctx context.Context, ev *QueryEvent)
}

// WithQueryHook registers h to run around every query/exec the DB issues.
func WithQueryHook(h QueryHook) Option {
	return func(o *dbOptions) {
		o.queryHooks = append(o.queryHooks, h)
	}
}

// WithQueryHookArgs opts into populating QueryEvent.Args for every
// QueryHook — off by default since query arguments may carry sensitive
// values a hook (e.g. one that logs) shouldn't see unless asked to.
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

// Dialect returns the dialect.Dialect this DB generates SQL for.
func (db *DB) Dialect() dialect.Dialect { return db.dialect }

// ExecContext runs a raw exec through the same path Create/Update/Delete
// use: query hooks fire around it and its error is classified via
// classifyError, unlike calling the underlying *sql.DB directly. op and
// model are reported to query hooks as QueryEvent.Operation/Model.
func (db *DB) ExecContext(ctx context.Context, op, model, sqlText string, args []any) (sql.Result, error) {
	return db.execContext(ctx, op, model, sqlText, args)
}

// QueryContext runs a raw query through the same path Find/First use:
// query hooks fire around it and its error is classified via
// classifyError, unlike calling the underlying *sql.DB directly. op and
// model are reported to query hooks as QueryEvent.Operation/Model.
func (db *DB) QueryContext(ctx context.Context, op, model, sqlText string, args []any) (*sql.Rows, error) {
	return db.queryContext(ctx, op, model, sqlText, args)
}
