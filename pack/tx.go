package pack

import (
	"context"
	"database/sql"
	"fmt"
	"sync/atomic"
)

type TransactionFunc func(tx *DB) error

type txScope interface {
	commit(ctx context.Context) error
	rollback(ctx context.Context) error
}

type rootTxScope struct{ tx *sql.Tx }

func (s rootTxScope) commit(ctx context.Context) error   { return s.tx.Commit() }
func (s rootTxScope) rollback(ctx context.Context) error { return s.tx.Rollback() }

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

func (db *DB) Commit() error {
	if db.scope == nil {
		return fmt.Errorf("pack: Commit: db is not a transaction returned by BeginTx")
	}

	return db.scope.commit(context.Background())
}

func (db *DB) Rollback() error {
	if db.scope == nil {
		return fmt.Errorf("pack: Rollback: db is not a transaction returned by BeginTx")
	}

	return db.scope.rollback(context.Background())
}

func (db *DB) InTransaction() bool {
	return db.scope != nil
}

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
