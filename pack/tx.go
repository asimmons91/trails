package pack

import (
	"context"
	"database/sql"
	"fmt"
)

type TransactionFunc func(tx *DB) error

func (db *DB) Tx(ctx context.Context, fn TransactionFunc) error {
	return db.txWithOptions(ctx, nil, fn)
}

func (db *DB) txWithOptions(ctx context.Context, opts *sql.TxOptions, fn TransactionFunc) error {
	if db.beginner == nil {
		return fn(db)
	}

	tx, err := db.beginner.BeginTx(ctx, opts)
	if err != nil {
		return err
	}

	txDB := &DB{conn: tx, dialect: db.dialect, opts: db.opts}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()

	if err = fn(txDB); err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}

func (db *DB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*DB, error) {
	if db.beginner == nil {
		return nil, fmt.Errorf("pack: BeginTx: already inside a transaction; savepoint-based nesting is not supported")
	}

	tx, err := db.beginner.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}

	return &DB{conn: tx, dialect: db.dialect, opts: db.opts, tx: tx}, nil
}

func (db *DB) Commit() error {
	if db.tx == nil {
		return fmt.Errorf("pack: Commit: db is not a transaction returned by BeginTx")
	}

	return db.tx.Commit()
}

func (db *DB) Rollback() error {
	if db.tx == nil {
		return fmt.Errorf("pack: Rollback: db is not a transaction returned by BeginTx")
	}

	return db.tx.Rollback()
}
