package sqlitedialect

import (
	"strconv"
	"strings"

	"github.com/asimmons91/trails/pack/dialect"
)

// SQLite implements dialect.Dialect. Error classification lives in
// pack/driver/sqlite, not here - see driver.ErrorDecoder.
type SQLite struct{}

var _ dialect.Dialect = SQLite{}

func New() SQLite { return SQLite{} }

func (SQLite) Name() string { return "sqlite" }

func (SQLite) QuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func (SQLite) Placeholder(n int) string { return "?" + strconv.Itoa(n) }

func (SQLite) SupportsILike() bool { return false }

func (SQLite) SupportsRowLocking() bool { return false }

func (SQLite) SupportsReturning() bool { return true }

func (SQLite) SupportsOnConflict() bool { return true }

func (SQLite) SupportsSavepoints() bool { return true }
