package pack

import (
	"fmt"

	"github.com/asimmons91/trails/pack/driver"
)

// Connect opens dsn via d.Open, wraps the result with d.Dialect() (see
// Open), and — if d also implements driver.ErrorDecoder — registers it as
// the DB's default error classifier so constraint violations come back as
// the Err*Violation types instead of the raw driver error. A WithErrorDecoder
// in opts overrides that default.
func Connect(d driver.Driver, dsn string, opts ...Option) (*DB, error) {
	sqlDB, err := d.Open(dsn)
	if err != nil {
		return nil, fmt.Errorf("pack: Connect: %w", err)
	}

	if ed, ok := d.(driver.ErrorDecoder); ok {
		opts = append([]Option{WithErrorDecoder(ed)}, opts...)
	}

	return Open(sqlDB, d.Dialect(), opts...), nil
}
