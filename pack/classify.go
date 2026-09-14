package pack

import "github.com/asimmons91/trails/pack/dialect"

// noopErrorDecoder is used when the Dialect passed to Open doesn't itself
// implement dialect.ErrorDecoder.
type noopErrorDecoder struct{}

func (noopErrorDecoder) Classify(err error) (string, bool) { return "", false }

func WithErrorDecoder(d dialect.ErrorDecoder) Option {
	return func(o *dbOptions) {
		o.errDecoder = d
	}
}

func classifyError(db *DB, op, model string, err error) error {
	if err == nil {
		return nil
	}

	code, ok := db.opts.errDecoder.Classify(err)
	if !ok {
		return err
	}

	var constraint, table, column string
	if dd, ok := db.opts.errDecoder.(dialect.DetailedErrorDecoder); ok {
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
	case dialect.CodeUnique:
		return &ErrUniqueViolation{base}
	case dialect.CodeForeignKey:
		return &ErrForeignKeyViolation{base}
	case dialect.CodeNotNull:
		return &ErrNotNullViolation{base}
	case dialect.CodeCheck:
		return &ErrCheckViolation{base}
	default:
		return err
	}
}
