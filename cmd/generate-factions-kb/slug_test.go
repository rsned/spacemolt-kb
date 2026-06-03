package main

import "testing"

func TestFactionSlug(t *testing.T) {
	const id = "deadbeef"
	cases := []struct{ tag, want string }{
		{"HEXC", "hexc"},        // already clean
		{" DB ", "db"},          // leading/trailing/inner spaces
		{"SRA!", "sra"},         // trailing punctuation
		{"[o7]", "o7"},          // surrounding brackets
		{"A B C", "a_b_c"},      // inner spaces become single underscores
		{"a--b__c", "a_b_c"},    // runs of non-alnum collapse to one underscore
		{"DMX7", "dmx7"},        // digits kept
		{"!!!", id},             // nothing usable -> id fallback
		{"", id},                // empty -> id fallback
		{"naïve", "na_ve"},      // non-ASCII letter dropped to underscore
	}
	for _, c := range cases {
		if got := factionSlug(c.tag, id); got != c.want {
			t.Errorf("factionSlug(%q) = %q, want %q", c.tag, got, c.want)
		}
	}
}
