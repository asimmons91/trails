package sqlbuild

func appendFresh[T any](base []T, more ...T) []T {
	out := make([]T, len(base)+len(more))
	copy(out, base)
	copy(out[len(base):], more)

	return out
}
