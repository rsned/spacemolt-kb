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
