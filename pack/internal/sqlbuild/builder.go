package sqlbuild

// appendFresh returns a fresh slice combining base and more, never
// mutating base's backing array. Every builder's chain methods use this
// (instead of append) so a *Builder returned by one call is unaffected by
// a later call chained from the same earlier value.
func appendFresh[T any](base []T, more ...T) []T {
	out := make([]T, len(base)+len(more))
	copy(out, base)
	copy(out[len(base):], more)

	return out
}
