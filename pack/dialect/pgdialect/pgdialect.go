package pgdialect

import (
	"errors"
	"strconv"
	"strings"

	"github.com/asimmons91/trails/pack/dialect"
)

type Postgres struct{}

var _ dialect.ErrorDecoder = Postgres{}

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

func (Postgres) Classify(err error) (string, bool) {
	var se interface{ SQLState() string }
	if !errors.As(err, &se) {
		return "", false
	}
	switch se.SQLState() {
	case "23505":
		return dialect.CodeUnique, true
	case "23503":
		return dialect.CodeForeignKey, true
	case "23502":
		return dialect.CodeNotNull, true
	case "23514":
		return dialect.CodeCheck, true
	default:
		return "", false
	}
}
