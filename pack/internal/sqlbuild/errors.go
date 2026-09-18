package sqlbuild

import "fmt"

// ErrRawPlaceholderOutOfRange is returned when a raw SQL fragment (from
// Raw, SetExpr, or SelectRaw-adjacent raw helpers) references a "$N"
// placeholder with no matching argument.
type ErrRawPlaceholderOutOfRange struct {
	Fragment string
	Index    int
	NumArgs  int
}

func (e *ErrRawPlaceholderOutOfRange) Error() string {
	return fmt.Sprintf(
		"sqlbuild: raw fragment %q references placeholder $%d but only %d argument(s) were given",
		e.Fragment, e.Index, e.NumArgs,
	)
}

// ErrConflictingLockClause is returned by SelectBuilder.Render when both
// ForUpdate and ForShare were called on the same query.
type ErrConflictingLockClause struct{}

func (e *ErrConflictingLockClause) Error() string {
	return "sqlbuild: a SELECT cannot request both FOR UPDATE and FOR SHARE"
}

// ErrILikeUnsupportedByDialect is returned when rendering an ILike
// predicate against a dialect whose Dialect.SupportsILike is false.
type ErrILikeUnsupportedByDialect struct {
	Dialect string
}

func (e *ErrILikeUnsupportedByDialect) Error() string {
	return fmt.Sprintf("sqlbuild: %s does not support ILIKE statements", e.Dialect)
}

// ErrSkipLockedRequiresLockClause is returned by SelectBuilder.Render when
// SkipLocked was called without also calling ForUpdate or ForShare.
type ErrSkipLockedRequiresLockClause struct{}

func (e *ErrSkipLockedRequiresLockClause) Error() string {
	return "sqlbuild: SKIP LOCKED requires FOR UPDATE or FOR SHARE"
}

// ErrRowLockingUnsupportedByDialect is returned by SelectBuilder.Render
// when ForUpdate or ForShare was called against a dialect whose
// Dialect.SupportsRowLocking is false.
type ErrRowLockingUnsupportedByDialect struct {
	Dialect string
}

func (e *ErrRowLockingUnsupportedByDialect) Error() string {
	return fmt.Sprintf("sqlbuild: %s does not support row locking", e.Dialect)
}

// ErrReturningUnsupportedByDialect is returned by InsertBuilder.Render
// when Returning was called against a dialect whose
// Dialect.SupportsReturning is false.
type ErrReturningUnsupportedByDialect struct {
	Dialect string
}

func (e *ErrReturningUnsupportedByDialect) Error() string {
	return fmt.Sprintf("sqlbuild: %s does not support INSERT ... RETURNING", e.Dialect)
}

// ErrDDLUnsupportedByDialect is returned by any DDL-rendering builder
// (AddColumn, AlterColumn, CreateTable, DropIndex, RenameIndex, ...) when
// the given dialect.Dialect doesn't also implement dialect.DDL.
type ErrDDLUnsupportedByDialect struct {
	Dialect string
}

func (d *ErrDDLUnsupportedByDialect) Error() string {
	return fmt.Sprintf("sqlbuild: %s does not implement dialect.DDL", d.Dialect)
}

// ErrConstraintsUnsupportedByDialect is returned by
// CreateConstraintBuilder/DropConstraintBuilder.Render for a dialect
// (SQLite) that can't add or drop a constraint after table creation.
type ErrConstraintsUnsupportedByDialect struct {
	Dialect string
}

func (c *ErrConstraintsUnsupportedByDialect) Error() string {
	return fmt.Sprintf("sqlbuild: %s does not support adding or dropping constraints after table creation", c.Dialect)
}
