package dialect

type Dialect interface {
	Name() string

	QuoteIdent(name string) string

	Placeholder(n int) string

	SupportsILike() bool

	SupportsRowLocking() bool

	SupportsReturning() bool

	SupportsOnConflict() bool

	SupportsSavepoints() bool
}

const (
	CodeUnique     = "ERR_UNIQUE"
	CodeForeignKey = "ERR_FOREIGN_KEY"
	CodeNotNull    = "ERR_NOT_NULL"
	CodeCheck      = "ERR_CHECK"
)

type ErrorDecoder interface {
	Classify(err error) (code string, ok bool)
}

type DetailedErrorDecoder interface {
	ErrorDecoder

	Detail(err error) (constraint, table, column string)
}
