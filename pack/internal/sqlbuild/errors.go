package sqlbuild

import "fmt"

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

type ErrConflictingLockClause struct{}

func (e *ErrConflictingLockClause) Error() string {
	return "sqlbuild: a SELECT cannot request both FOR UPDATE and FOR SHARE"
}

type ErrILikeUnsupportedByDialect struct {
	Dialect string
}

func (e *ErrILikeUnsupportedByDialect) Error() string {
	return fmt.Sprintf("sqlbuild: %s does not support ILIKE statements", e.Dialect)
}

type ErrRowLockingUnsupportedByDialect struct {
	Dialect string
}

func (e *ErrRowLockingUnsupportedByDialect) Error() string {
	return fmt.Sprintf("sqlbuild: %s does not support row locking", e.Dialect)
}

type ErrReturningUnsupportedByDialect struct {
	Dialect string
}

func (e *ErrReturningUnsupportedByDialect) Error() string {
	return fmt.Sprintf("sqlbuild: %s does not support INSERT ... RETURNING", e.Dialect)
}

type ErrDDLUnsupportedByDialect struct {
	Dialect string
}

func (d *ErrDDLUnsupportedByDialect) Error() string {
	return fmt.Sprintf("sqlbuild: %s does not implement dialect.DDL", d.Dialect)
}

type ErrConstraintsUnsupportedByDialect struct {
	Dialect string
}

func (c *ErrConstraintsUnsupportedByDialect) Error() string {
	return fmt.Sprintf("sqlbuild: %s does not support adding or dropping constraints after table creation", c.Dialect)
}
