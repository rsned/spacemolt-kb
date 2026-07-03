package main

import "strings"

// factionSlug builds a URL path segment from a faction tag: lowercased, with
// every run of non-[a-z0-9] characters collapsed to a single underscore and
// leading/trailing underscores trimmed (so " DB " -> "db", "SRA!" -> "sra",
// "[o7]" -> "o7"). Falls back to id when nothing usable remains. The game server
// enforces case-insensitive tag uniqueness, so distinct factions don't collide;
// loadFactions still guards against a normalization clash just in case.
func factionSlug(tag, id string) string {
	var b strings.Builder
	prevUnderscore := false
	for _, r := range strings.ToLower(tag) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevUnderscore = false
		default:
			if !prevUnderscore && b.Len() > 0 {
				b.WriteByte('_')
				prevUnderscore = true
			}
		}
	}
	if s := strings.Trim(b.String(), "_"); s != "" {
		return s
	}
	return id
}
