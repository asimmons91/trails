package pack

import (
	"context"
	"database/sql"
	"fmt"
	"sync/atomic"
)

// TransactionFunc is the callback DB.Tx runs inside a transaction.
type TransactionFunc func(tx *DB) error

// txScope abstracts committing/rolling back a transaction, whether it's a
// root *sql.Tx (rootTxScope) or a nested SAVEPOINT (savepointScope).
type txScope interface {
	commit(ctx context.Context) error
	rollback(ctx context.Context) error
}

// rootTxScope commits/rolls back a top-level *sql.Tx, returned by BeginTx
// when the DB wasn't already inside a transaction.
type rootTxScope struct{ tx *sql.Tx }

func (s rootTxScope) commit(ctx context.Context) error   { return s.tx.Commit() }
func (s rootTxScope) rollback(ctx context.Context) error { return s.tx.Rollback() }

// savepointScope commits/rolls back a nested SAVEPOINT, returned by
// BeginTx when the DB was already inside a transaction.
type savepointScope struct {
	db   *DB
	name string
}

func (s savepointScope) commit(ctx context.Context) error {
	_, err := s.db.execContext(ctx, "RELEASE", "", "RELEASE SAVEPOINT "+s.name, nil)
	return err
}

func (s savepointScope) rollback(ctx context.Context) error {
	_, err := s.db.execContext(ctx, "ROLLBACK TO SAVEPOINT", "", "ROLLBACK TO SAVEPOINT "+s.name, nil)
	return err
}

// Tx runs fn inside a transaction (via BeginTx), committing if fn returns
// nil and rolling back otherwise. A panic inside fn also rolls back, then
// re-panics.
func (db *DB) Tx(ctx context.Context, fn TransactionFunc) error {
	return db.txWithOptions(ctx, nil, fn)
}

func (db *DB) txWithOptions(ctx context.Context, opts *sql.TxOptions, fn TransactionFunc) error {
	txDB, err := db.BeginTx(ctx, opts)
	if err != nil {
		return err
	}

	defer func() {
		if p := recover(); p != nil {
			_ = txDB.Rollback()
			panic(p)
		}
	}()

	if err := fn(txDB); err != nil {
		_ = txDB.Rollback()
		return err
	}

	return txDB.Commit()
}

// BeginTx starts a transaction and returns a *DB scoped to it — pass it to
// the same query/write functions as any other DB. If db is a root
// connection, this starts a top-level *sql.Tx. If db is already inside a
// transaction (itself the result of BeginTx), this instead creates a
// nested SAVEPOINT, requiring dialect.Dialect.SupportsSavepoints; the
// returned *DB's Commit/Rollback then release or roll back to that
// savepoint rather than ending the outer transaction. Pair with
// Commit/Rollback, or use Tx to handle that automatically.
func (db *DB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*DB, error) {
	if db.beginner != nil {
		tx, err := db.beginner.BeginTx(ctx, opts)
		if err != nil {
			return nil, err
		}

		seq := new(atomic.Int64)
		return &DB{conn: tx, dialect: db.dialect, opts: db.opts, scope: rootTxScope{tx: tx}, savepointSeq: seq}, nil
	}

	if !db.dialect.SupportsSavepoints() {
		return nil, fmt.Errorf("pack: BeginTx: already inside a transaction; %s does not support savepoints", db.dialect.Name())
	}

	name := fmt.Sprintf("pack_sp%d", db.savepointSeq.Add(1))

	nested := &DB{conn: db.conn, dialect: db.dialect, opts: db.opts, savepointSeq: db.savepointSeq}
	if _, err := nested.execContext(ctx, "SAVEPOINT", "", "SAVEPOINT "+name, nil); err != nil {
		return nil, err
	}
	nested.scope = savepointScope{db: nested, name: name}

	return nested, nil
}

// Commit commits the transaction db was returned by BeginTx as — a plain
// commit for a root transaction, or RELEASE SAVEPOINT for a nested one.
// Errors if db isn't a *DB BeginTx returned.
func (db *DB) Commit() error {
	if db.scope == nil {
		return fmt.Errorf("pack: Commit: db is not a transaction returned by BeginTx")
	}

	return db.scope.commit(context.Background())
}

// Rollback rolls back the transaction db was returned by BeginTx as — a
// plain rollback for a root transaction, or ROLLBACK TO SAVEPOINT for a
// nested one. Errors if db isn't a *DB BeginTx returned.
func (db *DB) Rollback() error {
	if db.scope == nil {
		return fmt.Errorf("pack: Rollback: db is not a transaction returned by BeginTx")
	}

	return db.scope.rollback(context.Background())
}

// InTransaction reports whether db was returned by BeginTx (root or
// nested savepoint).
func (db *DB) InTransaction() bool {
	return db.scope != nil
}

// PinnedConn runs fn against a *DB pinned to a single underlying
// connection for fn's duration, released afterward. Use it for operations
// that must all run on the same connection outside of pooling — e.g.
// pack/migrate's SQLite table-rebuild, which needs a session-scoped PRAGMA
// plus a transaction on that same connection. Only a root DB (from Open
// or Connect) supports pinning; calling PinnedConn on a DB already
// returned by BeginTx or PinnedConn itself returns
// ErrPinnedConnUnsupported.
func (db *DB) PinnedConn(ctx context.Context, fn func(pinned *DB) error) error {
	pool, ok := db.beginner.(interface {
		Conn(ctx context.Context) (*sql.Conn, error)
	})
	if !ok {
		return ErrPinnedConnUnsupported
	}

	conn, err := pool.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = conn.Close()
	}()

	pinned := &DB{conn: conn, beginner: conn, dialect: db.dialect, opts: db.opts, savepointSeq: new(atomic.Int64)}
	return fn(pinned)
}
