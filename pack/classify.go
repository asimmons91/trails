package pack

import "github.com/asimmons91/trails/pack/driver"

// noopErrorDecoder is used when the Dialect passed to Open doesn't itself
// implement driver.ErrorDecoder.
type noopErrorDecoder struct{}

func (noopErrorDecoder) Classify(err error) (string, bool) { return "", false }

func WithErrorDecoder(d driver.ErrorDecoder) Option {
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
	default:
		return err
	}
}
