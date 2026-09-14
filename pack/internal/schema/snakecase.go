package schema

import (
	"strings"
	"unicode"
)

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
