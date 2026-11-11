package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMapSystemDescriptions(t *testing.T) {
	dir := t.TempDir()
	js := `{"systems":[{"system_id":"sol","description":"  Home.  "},{"system_id":"void","description":""}],"total_count":2}`
	if err := os.WriteFile(filepath.Join(dir, "get_map.json"), []byte(js), 0o644); err != nil {
		t.Fatal(err)
	}
	got := mapSystemDescriptions(dir)
	if len(got) != 1 || got["sol"] != "Home." {
		t.Fatalf("descriptions %q, want only sol=Home.", got)
	}
	if mapSystemDescriptions(t.TempDir()) != nil {
		t.Fatal("missing get_map.json should yield nil")
	}
}

// A system agents have visited but the map scrape leaves out is uncharted
// (v0.687: uncharted frontier systems are excluded from maps and search).
func TestMarkUncharted(t *testing.T) {
	a, b := &System{ID: "sol"}, &System{ID: "beyond"}
	markUncharted([]*System{a, b}, map[string]bool{"sol": true})
	if a.Uncharted || !b.Uncharted {
		t.Fatalf("sol=%v beyond=%v", a.Uncharted, b.Uncharted)
	}
	// With no map scrape at all, nothing can be judged uncharted.
	c := &System{ID: "beyond"}
	markUncharted([]*System{c}, nil)
	if c.Uncharted {
		t.Fatal("marked uncharted without a map")
	}
}
