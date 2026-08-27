package kblegacy

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "legacy.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// An entry the devs deleted outright carries the patch that did it, so pages can
// say holdings were refunded rather than "existing ones still work".
func TestRemovedEntry(t *testing.T) {
	p := write(t, `{"items":{
	  "point_defense_turret":{"name":"Point Defense Turret","last_in_catalog":"20260308",
	    "removed":{"patch":"v0.566.0","date":"2026-08-27","refund":"10x former base value in credits"}},
	  "chaff_bundle":{"name":"Chaff Bundle","last_in_catalog":"20260807"}}}`)
	s, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}

	e, ok := s.Item("point_defense_turret")
	if !ok {
		t.Fatal("point_defense_turret not found")
	}
	if !e.WasRemoved() {
		t.Error("WasRemoved() = false, want true for an entry with a removed block")
	}
	if e.Removed.Patch != "v0.566.0" {
		t.Errorf("Patch = %q, want v0.566.0", e.Removed.Patch)
	}
	if e.Removed.Refund != "10x former base value in credits" {
		t.Errorf("Refund = %q", e.Removed.Refund)
	}

	// A plain legacy entry is merely unpurchasable; it must not read as deleted.
	c, ok := s.Item("chaff_bundle")
	if !ok {
		t.Fatal("chaff_bundle not found")
	}
	if c.WasRemoved() {
		t.Error("WasRemoved() = true for an entry with no removed block")
	}
	if c.Removed != nil {
		t.Errorf("Removed = %+v, want nil", c.Removed)
	}
}

// Date formats the snapshot directory name; the removal carries its own date.
func TestDate(t *testing.T) {
	if got := (Entry{LastInCatalog: "20260308"}).Date(); got != "2026-03-08" {
		t.Errorf("Date() = %q, want 2026-03-08", got)
	}
	if got := (Entry{}).Date(); got != "" {
		t.Errorf("Date() = %q, want empty", got)
	}
}
