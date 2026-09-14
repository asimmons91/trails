package sqlitedialect

import (
	"errors"
	"strconv"
	"strings"

	"github.com/asimmons91/trails/pack/dialect"
)

type SQLite struct{}

var _ dialect.ErrorDecoder = SQLite{}

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

func (SQLite) Classify(err error) (string, bool) {
	var ce interface{ Code() int }
	if !errors.As(err, &ce) {
		return "", false
	}
	switch ce.Code() {
	case 2067, 1555: // SQLITE_CONSTRAINT_UNIQUE, SQLITE_CONSTRAINT_PRIMARYKEY
		return dialect.CodeUnique, true
	case 787: // SQLITE_CONSTRAINT_FOREIGNKEY
		return dialect.CodeForeignKey, true
	case 1299: // SQLITE_CONSTRAINT_NOTNULL
		return dialect.CodeNotNull, true
	case 275: // SQLITE_CONSTRAINT_CHECK
		return dialect.CodeCheck, true
	default:
		return "", false
	}
}
