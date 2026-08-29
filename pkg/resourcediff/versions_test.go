package resourcediff

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadVersionFeed(t *testing.T) {
	dir := t.TempDir()
	write := func(date, body string) {
		if err := os.MkdirAll(filepath.Join(dir, date), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, date, "get_version.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("20260825", `{"version":"0.559.1","release_date":"2026-08-26","versions":[
		{"version":"0.559.1","release_date":"2026-08-26","notes":["Fixes to combat edge cases."]},
		{"version":"0.559.0","release_date":"2026-08-25","notes":["Something else."]}]}`)
	write("20260827", `{"version":"0.566.2","release_date":"2026-08-27","versions":[
		{"version":"0.566.2","release_date":"2026-08-27","notes":["Modules apply effects."]},
		{"version":"0.566.0","release_date":"2026-08-27","notes":["**11 newly available mineable resources** have been placed across the galaxy: six ores, four ices, and nebulium crystals.","**9 new manufactured items** expand the trees."]},
		{"version":"0.559.1","release_date":"2026-08-26","notes":["Fixes to combat edge cases."]}]}`)
	// Non-date directories and a symlink-style "latest" copy are ignored.
	write("latest", `{"version":"0.566.2","versions":[]}`)

	feed, err := LoadVersionFeed(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := feed.Current(); got != "0.566.2" {
		t.Errorf("Current = %q", got)
	}
	if len(feed.Patches) != 4 {
		t.Fatalf("patches = %+v", feed.Patches)
	}
	// Newest first.
	if feed.Patches[0].Version != "0.566.2" || feed.Patches[3].Version != "0.559.0" {
		t.Errorf("order = %+v", feed.Patches)
	}

	// Resource patches strictly after 0.559.1 up to and including 0.566.2.
	got := feed.ResourcePatches("0.559.1", "0.566.2")
	if len(got) != 1 || got[0].Version != "0.566.0" {
		t.Fatalf("ResourcePatches = %+v", got)
	}
	if len(got[0].ResourceNotes()) != 1 || got[0].ResourceNotes()[0] != "11 newly available mineable resources have been placed across the galaxy: six ores, four ices, and nebulium crystals." {
		t.Errorf("ResourceNotes = %q", got[0].ResourceNotes())
	}
	// An empty lower bound means "everything up to".
	if got := feed.ResourcePatches("", "0.566.2"); len(got) != 1 {
		t.Errorf("open-ended ResourcePatches = %+v", got)
	}
	// Lookup by version.
	if p := feed.Patch("0.566.0"); p == nil || p.ReleaseDate != "2026-08-27" {
		t.Errorf("Patch(0.566.0) = %+v", p)
	}
	if feed.Patch("9.9.9") != nil {
		t.Error("unknown version should be nil")
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.566.2", "0.566.0", 1}, {"0.559.1", "0.566.0", -1}, {"0.566.0", "0.566.0", 0}, {"1.0.0", "0.999.9", 1}, {"", "0.1.0", -1},
	}
	for _, c := range cases {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestResourceNotes_IgnoresItemNames(t *testing.T) {
	p := Patch{Notes: []string{
		"Modules: Force Field Generator II, Ore Compressor, and Voidborn Phase Cloak.",
		"Phase Crystal drops were rebalanced.",
		"Six ores and four ices were placed across the galaxy.",
		"Mining yield at depleted deposits is lower.",
	}}
	got := p.ResourceNotes()
	if len(got) != 2 || got[0] != "Six ores and four ices were placed across the galaxy." || got[1] != "Mining yield at depleted deposits is lower." {
		t.Errorf("ResourceNotes = %q", got)
	}
}
