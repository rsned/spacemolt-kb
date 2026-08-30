package wildlife

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCodex(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "codex.json")
	if err := os.WriteFile(p, []byte(`{"_note":"x","belt_grazer":{"description":"Hand-recorded.","scanned_tick":5,"scanned_utc":"2026-08-29","source":"scan"},"ghost":{"description":"Boo."}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := LoadCodex(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(c) != 2 || c["belt_grazer"].Description != "Hand-recorded." || c["belt_grazer"].ScannedTick != 5 {
		t.Errorf("codex = %+v", c)
	}
	species := []Species{{ID: "belt_grazer", Description: "From the DB."}, {ID: "ghost"}, {ID: "other"}}
	c.Apply(species)
	if species[0].Description != "From the DB." || species[0].CodexSource != "db" {
		t.Errorf("DB description must win: %+v", species[0])
	}
	if species[1].Description != "Boo." || species[1].CodexSource != "codex" {
		t.Errorf("codex fallback: %+v", species[1])
	}
	if species[2].Description != "" {
		t.Errorf("unknown species untouched: %+v", species[2])
	}
	// A missing file is an empty codex, not an error.
	if c, err := LoadCodex(filepath.Join(dir, "nope.json")); err != nil || len(c) != 0 {
		t.Errorf("missing file: %v %+v", err, c)
	}
}
