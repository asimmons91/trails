package pack

import "github.com/asimmons91/trails/pack/driver"

// noopErrorDecoder is used when the Dialect passed to Open doesn't itself
// implement driver.ErrorDecoder.
type noopErrorDecoder struct{}

func (noopErrorDecoder) Classify(err error) (string, bool) { return "", false }

// WithErrorDecoder overrides the driver.ErrorDecoder Connect/Open would
// otherwise use for classifying database errors into the Err*Violation
// types (see classifyError).
func WithErrorDecoder(d driver.ErrorDecoder) Option {
	return func(o *dbOptions) {
		o.errDecoder = d
	}
}

// classifyError turns a raw driver error into one of the Err*Violation
// types via db's configured driver.ErrorDecoder, or returns err unchanged
// if the decoder doesn't recognize it (or err is nil).
func classifyError(db *DB, op, model string, err error) error {
	if err == nil {
		return nil
	}

	code, ok := db.opts.errDecoder.Classify(err)
	if !ok {
		return err
	}

	var constraint, table, column string
	if dd, ok := db.opts.errDecoder.(driver.DetailedErrorDecoder); ok {
		constraint, table, column = dd.Detail(err)
	}

	base := violation{
		Model:      model,
		Operation:  op,
		Constraint: constraint,
		Table:      table,
		Column:     column,
		err:        err,
	}

	switch code {
	case driver.CodeUnique:
		return &ErrUniqueViolation{base}
	case driver.CodeForeignKey:
		return &ErrForeignKeyViolation{base}
	case driver.CodeNotNull:
		return &ErrNotNullViolation{base}
	case driver.CodeCheck:
		return &ErrCheckViolation{base}
	case driver.CodeSerializationFailure:
		return &ErrSerializationFailure{base}
	default:
		return err
	}
}
