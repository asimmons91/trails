package pack

import (
	"database/sql"
	"fmt"
)

var ErrNoRows = fmt.Errorf("pack: no rows in result set: %w", sql.ErrNoRows)

var ErrPinnedConnUnsupported = fmt.Errorf("pack: PinnedConn: underlying connection source does not support connection pinning")

type ErrUintOverflow struct {
	Model  string
	Field  string
	Column string
	Value  uint64
}

func (e *ErrUintOverflow) Error() string {
	return fmt.Sprintf(
		"pack: %s.%s (column %q): value %d exceeds math.MaxInt64, no bigint representation",
		e.Model, e.Field, e.Column, e.Value,
	)
}

type ErrZeroCompositeKey struct {
	Model string
}

func (e *ErrZeroCompositeKey) Error() string {
	return fmt.Sprintf(
		"pack: Create(%s): composite primary key is the zero value; composite keys are never server-generated and must be set before insert",
		e.Model,
	)
}

type ErrSetOperationBlockedByHooks struct {
	Model     string
	Operation string
}

func (e *ErrSetOperationBlockedByHooks) Error() string {
	return fmt.Sprintf(
		"pack: %s implements a hook relevant to a set-based %s, which cannot fire (no Go instance exists per row); call .SkipHooks() to proceed anyway",
		e.Model, e.Operation,
	)
}

type violation struct {
	Model      string
	Operation  string
	Constraint string
	Table      string
	Column     string
	err        error
}

func (e *violation) Unwrap() error { return e.err }

type ErrUniqueViolation struct{ violation }

func (e *ErrUniqueViolation) Error() string {
	return fmt.Sprintf("pack: %s.%s: unique constraint %q violated", e.Model, e.Operation, e.Constraint)
}

type ErrForeignKeyViolation struct{ violation }

func (e *ErrForeignKeyViolation) Error() string {
	return fmt.Sprintf("pack: %s.%s: foreign key constraint %q violated", e.Model, e.Operation, e.Constraint)
}

type ErrNotNullViolation struct{ violation }

func (e *ErrNotNullViolation) Error() string {
	return fmt.Sprintf("pack: %s.%s: foreign key constraint %q violated", e.Model, e.Operation, e.Constraint)
}

type ErrCheckViolation struct{ violation }

func (e *ErrCheckViolation) Error() string {
	return fmt.Sprintf("pack: %s.%s: check constraint %q violated", e.Model, e.Operation, e.Constraint)
}

// ErrSerializationFailure indicates the database aborted a statement
// because of contention with a concurrent transaction — a deadlock, or a
// serializable-isolation conflict — rather than any fault of the
// statement itself. Unlike the constraint-violation errors above,
// retrying the whole transaction from scratch is the correct, expected
// response (the database's own error text says as much, e.g. MySQL's
// "Deadlock found when trying to get lock; try restarting transaction").
type ErrSerializationFailure struct{ violation }

func (e *ErrSerializationFailure) Error() string {
	return fmt.Sprintf("pack: %s.%s: aborted by the database due to a concurrent transaction conflict: %v", e.Model, e.Operation, e.err)
}
