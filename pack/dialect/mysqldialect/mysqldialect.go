package mysqldialect

import (
	"strings"

	"github.com/asimmons91/trails/pack/dialect"
)

// MySQL implements dialect.Dialect. Error classification lives in
// pack/driver/mysql, not here - see driver.ErrorDecoder.
type MySQL struct{}

var _ dialect.Dialect = MySQL{}

func New() MySQL { return MySQL{} }

func (MySQL) Name() string { return "mysql" }

func (MySQL) QuoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

func (MySQL) Placeholder(int) string { return "?" }

func (MySQL) SupportsILike() bool { return false }

func (MySQL) SupportsRowLocking() bool { return true }

func (MySQL) SupportsReturning() bool { return false }

func (MySQL) SupportsOnConflict() bool { return false }

func (MySQL) SupportsSavepoints() bool { return true }
