package profilejson

import (
	"strings"
	"unicode"
)

// PlanetSlug normalizes a planet name into a filename-safe slug
// matching the regex [a-z0-9_]+. Non-alphanumeric runes are
// treated as separators; consecutive separators collapse into a
// single underscore; leading and trailing underscores are
// trimmed.
func PlanetSlug(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	prevSep := true
	for _, r := range strings.ToLower(name) {
		switch {
		case unicode.IsLetter(r) && r < 128, unicode.IsDigit(r) && r < 128:
			b.WriteRune(r)
			prevSep = false
		default:
			if !prevSep {
				b.WriteByte('_')
				prevSep = true
			}
		}
	}
	out := b.String()
	return strings.Trim(out, "_")
}
