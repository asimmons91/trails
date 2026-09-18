package schema

import (
	"strings"
	"unicode"
)

// toSnakeCase is the default column/FK-name derivation from a Go
// identifier: a new word boundary (and thus an inserted "_") starts at an
// uppercase rune following a lowercase or digit, or at the last uppercase
// rune of a run that's followed by a lowercase one (so "UserID" ->
// "user_id", not "user_i_d").
func toSnakeCase(s string) string {
	if s == "" {
		return s
	}

	runes := []rune(s)
	var b strings.Builder
	b.Grow(len(runes) + 4)

	for i, r := range runes {
		if unicode.IsUpper(r) && i > 0 {
			prev := runes[i-1]
			if (unicode.IsLower(prev) || unicode.IsDigit(prev)) ||
				(unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1])) {
				b.WriteByte('_')
			}
		}

		b.WriteRune(unicode.ToLower(r))
	}

	return b.String()
}
