package main

import (
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// slugify lowercases s, strips diacritics, and replaces every run of
// non-alphanumeric characters with a single '-', trimming leading/trailing '-'.
// Returns "" when nothing usable remains.
func slugify(s string) string {
	// Decompose accents (ö -> o) then drop combining marks.
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	if out, _, err := transform.String(t, s); err == nil {
		s = out
	}
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// playerSlug builds a stable, unique slug from a username and player id:
// "<slugified-username>-<first 8 of id>". Falls back to just the id8 when the
// username slugifies to empty.
func playerSlug(username, playerID string) string {
	id8 := playerID
	if len(id8) > 8 {
		id8 = id8[:8]
	}
	base := slugify(username)
	if base == "" {
		return id8
	}
	return base + "-" + id8
}
