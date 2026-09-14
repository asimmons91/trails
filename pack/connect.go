package pack

import (
	"fmt"

	"github.com/asimmons91/trails/pack/driver"
)

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
