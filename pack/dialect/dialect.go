// Package dialect defines the per-database-engine capability surface
// (Dialect) and DDL-generation surface (DDL) that pack, pack/migrate, and
// pack/internal/sqlbuild are written against, so the same query- and
// DDL-building code works across Postgres, MySQL, and SQLite. Each engine
// has its own implementation package (pgdialect, mysqldialect,
// sqlitedialect).
package dialect

// Dialect is the per-engine SQL syntax and capability surface that pack's
// query builder is written against.
type Dialect interface {
	// Name identifies the dialect, e.g. in error messages.
	Name() string

	// QuoteIdent quotes name for safe use as an identifier (table/column)
	// in generated SQL.
	QuoteIdent(name string) string

	// Placeholder renders the nth (1-indexed) bind placeholder in this
	// dialect's syntax, e.g. "$1" for Postgres or "?" for MySQL.
	Placeholder(n int) string

	// SupportsILike reports whether the dialect has a native
	// case-insensitive LIKE. Rendering an ILike predicate errors if it
	// doesn't.
	SupportsILike() bool

	// SupportsRowLocking reports whether the dialect supports SELECT ...
	// FOR UPDATE/FOR SHARE. Rendering a locked SELECT errors if it doesn't.
	SupportsRowLocking() bool

	// SupportsReturning reports whether the dialect can return the
	// inserted/updated row from an INSERT/UPDATE statement. Create and
	// CreateAll fall back to a follow-up SELECT when it can't.
	SupportsReturning() bool

	// SupportsOnConflict reports whether the dialect supports an
	// INSERT ... ON CONFLICT clause. OnConflict errors if it doesn't.
	SupportsOnConflict() bool

	// SupportsSavepoints reports whether the dialect supports nested
	// transactions via SAVEPOINT. DB.BeginTx errors if it doesn't and a
	// transaction is already open.
	SupportsSavepoints() bool
}
