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
