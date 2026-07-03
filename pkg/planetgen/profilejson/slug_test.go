package profilejson

import "testing"

func TestPlanetSlug(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"simple ascii", "Earth", "earth"},
		{"spaces become underscores", "82 Eridani II", "82_eridani_ii"},
		{"hyphens become underscores", "Alpha-Centauri-Bb", "alpha_centauri_bb"},
		{"multiple separators collapse", "  Foo--Bar  ", "foo_bar"},
		{"strip non-alnum", "Wolf 359 ψ", "wolf_359"},
		{"empty stays empty", "", ""},
		{"already-slug passthrough", "terran_default", "terran_default"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PlanetSlug(tc.in); got != tc.want {
				t.Errorf("PlanetSlug(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
