package pgdialect

import (
	"strconv"
	"strings"

	"github.com/asimmons91/trails/pack/dialect"
)

// Postgres implements dialect.Dialect. Error classification lives in
// pack/driver/postgres, not here - see driver.ErrorDecoder.
type Postgres struct{}

var _ dialect.Dialect = Postgres{}

func New() Postgres { return Postgres{} }

func (Postgres) Name() string { return "postgres" }

func (Postgres) QuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func (Postgres) Placeholder(n int) string {
	return "$" + strconv.Itoa(n)
}

func (Postgres) SupportsILike() bool { return true }

func (Postgres) SupportsRowLocking() bool { return true }

func (Postgres) SupportsReturning() bool { return true }

func (Postgres) SupportsOnConflict() bool { return true }

func (Postgres) SupportsSavepoints() bool { return true }
