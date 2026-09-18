package pack

import (
	"database/sql"
	"fmt"
)

// ErrNoRows is returned by Query[T].First (and wraps sql.ErrNoRows) when
// no row matches.
var ErrNoRows = fmt.Errorf("pack: no rows in result set: %w", sql.ErrNoRows)

// ErrPinnedConnUnsupported is returned by DB.PinnedConn when the DB's
// underlying connection source can't pin a single connection.
var ErrPinnedConnUnsupported = fmt.Errorf("pack: PinnedConn: underlying connection source does not support connection pinning")

// ErrUintOverflow is returned when a field's unsigned value is too large to
// round-trip through the int64 pack binds it as.
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

// ErrZeroCompositeKey is returned by Create when a model with a composite
// primary key is inserted with that key still at its zero value —
// composite keys are never server-generated, so this always indicates a
// caller mistake.
type ErrZeroCompositeKey struct {
	Model string
}

func (e *ErrZeroCompositeKey) Error() string {
	return fmt.Sprintf(
		"pack: Create(%s): composite primary key is the zero value; composite keys are never server-generated and must be set before insert",
		e.Model,
	)
}

// ErrSetOperationBlockedByHooks is returned by a set-based Query[T].Update
// or Query[T].Delete/DeleteAll when the model implements a hook relevant to
// that operation: a set-based statement affects rows without ever
// materializing a Go instance for each one, so the hook has nothing to run
// against. Call Query[T].SkipHooks to bypass this and run the statement
// anyway.
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

// violation is the shared base of every constraint-violation error below:
// classifyError fills it in from the driver.ErrorDecoder's classification
// (and, if it's a driver.DetailedErrorDecoder, the offending
// constraint/table/column).
type violation struct {
	Model      string
	Operation  string
	Constraint string
	Table      string
	Column     string
	err        error
}

func (e *violation) Unwrap() error { return e.err }

// ErrUniqueViolation indicates a write would have violated a unique
// constraint.
type ErrUniqueViolation struct{ violation }

func (e *ErrUniqueViolation) Error() string {
	return fmt.Sprintf("pack: %s.%s: unique constraint %q violated", e.Model, e.Operation, e.Constraint)
}

// ErrForeignKeyViolation indicates a write would have violated a foreign
// key constraint.
type ErrForeignKeyViolation struct{ violation }

func (e *ErrForeignKeyViolation) Error() string {
	return fmt.Sprintf("pack: %s.%s: foreign key constraint %q violated", e.Model, e.Operation, e.Constraint)
}

// ErrNotNullViolation indicates a write would have left a not-null column
// without a value.
type ErrNotNullViolation struct{ violation }

func (e *ErrNotNullViolation) Error() string {
	return fmt.Sprintf("pack: %s.%s: not-null constraint %q violated", e.Model, e.Operation, e.Constraint)
}

// ErrCheckViolation indicates a write would have violated a check
// constraint.
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
