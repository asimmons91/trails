package testdb

import (
	"context"
	"database/sql/driver"
	"errors"
	"io"
)

type fakeConnector struct {
	db *FakeDB
}

func (c *fakeConnector) Connect(context.Context) (driver.Conn, error) {
	return &fakeConn{db: c.db}, nil
}

func (c *fakeConnector) Driver() driver.Driver { return fakeDriver{} }

// fakeDriver exists only to satisfy driver.Connector.Driver; testdb
// connections are always created via FakeDB.Open, never sql.Open.
type fakeDriver struct{}

func (fakeDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("testdb: connect via FakeDB.Open(), not sql.Open")
}

type fakeConn struct {
	db *FakeDB
}

func (c *fakeConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("testdb: unsupported; QueryContext/ExecContext handle everything")
}

func (c *fakeConn) Close() error { return nil }

func (c *fakeConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

// BeginTx implements driver.ConnBeginTx, which database/sql prefers over
// Begin when present — this is what lets context and *sql.TxOptions reach
// the fake driver at all (R10.3).
func (c *fakeConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	c.db.recordTxBegin(opts)
	return &fakeTx{db: c.db}, nil
}

type fakeTx struct{ db *FakeDB }

func (t *fakeTx) Commit() error {
	t.db.recordTxEnd("COMMIT")
	return nil
}

func (t *fakeTx) Rollback() error {
	t.db.recordTxEnd("ROLLBACK")
	return nil
}

// CheckNamedValue disables database/sql's DefaultParameterConverter so
// captured Args preserve the exact Go type/value passed at the call site
// (e.g. a literal int stays an int, not int64) — needed for byte-exact
// golden argument-slice comparisons.
func (c *fakeConn) CheckNamedValue(nv *driver.NamedValue) error { return nil }

func namedValuesToArgs(args []driver.NamedValue) []any {
	out := make([]any, len(args))
	for i, a := range args {
		out[i] = a.Value
	}
	return out
}

func (c *fakeConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.db.record(query, namedValuesToArgs(args))
	res, err := c.db.next()
	if err != nil {
		return nil, err
	}
	if res.Err != nil {
		return nil, res.Err
	}
	return &fakeRows{db: c.db, cols: res.Columns, data: res.Rows}, nil
}

func (c *fakeConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.db.record(query, namedValuesToArgs(args))
	res, err := c.db.next()
	if err != nil {
		return nil, err
	}
	if res.Err != nil {
		return nil, res.Err
	}
	return fakeResult{lastID: res.LastInsertID, affected: res.RowsAffected}, nil
}

type fakeRows struct {
	db   *FakeDB
	cols []string
	data [][]driver.Value
	pos  int
}

func (r *fakeRows) Columns() []string { return r.cols }

func (r *fakeRows) Close() error {
	r.db.recordRowsClosed()
	return nil
}

func (r *fakeRows) Next(dest []driver.Value) error {
	if r.pos >= len(r.data) {
		return io.EOF
	}
	copy(dest, r.data[r.pos])
	r.pos++
	return nil
}

type fakeResult struct {
	lastID   int64
	affected int64
}

func (r fakeResult) LastInsertId() (int64, error) { return r.lastID, nil }
func (r fakeResult) RowsAffected() (int64, error) { return r.affected, nil }
