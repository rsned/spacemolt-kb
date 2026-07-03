package gamediff

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFacilities_MergesCategoryFiles(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("facility_production.json", `{"category":"production","types":[{"id":"forge","name":"Forge"}]}`)
	write("facility_service.json", `{"category":"service","types":[{"id":"clinic","name":"Clinic"},{"id":"bar","name":"Bar"}]}`)

	data, err := LoadFacilities(dir)
	if err != nil {
		t.Fatal(err)
	}
	// The merged blob must be diffable by DiffCatalog, keyed by "id".
	res, err := DiffCatalog([]byte(`{"items":[]}`), data)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Additions) != 3 {
		t.Fatalf("want 3 merged facilities, got %d: %v", len(res.Additions), res.Additions)
	}
}

func TestLoadFacilities_PrefersUnifiedCatalog(t *testing.T) {
	dir := t.TempDir()
	// New-format unified catalog plus a stale per-category file. The unified
	// file must win.
	if err := os.WriteFile(filepath.Join(dir, FacilityCatalogFile),
		[]byte(`{"items":[{"id":"unified","name":"Unified"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "facility_production.json"),
		[]byte(`{"types":[{"id":"stale","name":"Stale"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	data, err := LoadFacilities(dir)
	if err != nil {
		t.Fatal(err)
	}
	res, err := DiffCatalog([]byte(`{"items":[]}`), data)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Additions) != 1 || res.Additions[0].ID != "unified" {
		t.Fatalf("want only the unified entry, got %v", res.Additions)
	}
}

func TestLoadFacilities_NoData(t *testing.T) {
	if _, err := LoadFacilities(t.TempDir()); err == nil {
		t.Fatal("want error when no facility data present")
	}
}
