package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/profilejson"
)

// TestWriteOneMatchesEncodeDirect verifies that the seeder writes
// byte-identical output to what profilejson.Encode produces against
// GetProfile(type) — i.e., the seeder is a thin wrapper, not a
// reinterpretation of the profile.
func TestWriteOneMatchesEncodeDirect(t *testing.T) {
	dir := t.TempDir()
	spec := planetSpec{planetType: "terran", seed: "terran_default"}
	if _, err := writeOne(dir, spec, false); err != nil {
		t.Fatalf("writeOne: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "terran_default.json"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := profilejson.Encode(&profilejson.Envelope{
		SchemaVersion: profilejson.CurrentSchemaVersion,
		Type:          "terran",
		Seed:          "terran_default",
		HandTuned:     false,
		Profile:       planetgen.GetProfile("terran"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("seeder output differs from profilejson.Encode of same envelope")
	}
}

// TestWriteOneIdempotent: rerunning produces no changes and no error.
func TestWriteOneIdempotent(t *testing.T) {
	dir := t.TempDir()
	spec := planetSpec{planetType: "scorched", seed: "scorched_default"}
	written1, err := writeOne(dir, spec, false)
	if err != nil || !written1 {
		t.Fatalf("first write: written=%v err=%v", written1, err)
	}
	written2, err := writeOne(dir, spec, false)
	if err != nil {
		t.Fatalf("second write: %v", err)
	}
	if written2 {
		t.Errorf("second write should have skipped existing file")
	}
}

// TestWriteOneForceOverwrites: -force replaces existing content.
func TestWriteOneForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "terran_default.json")
	if err := os.WriteFile(path, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := planetSpec{planetType: "terran", seed: "terran_default"}
	if _, err := writeOne(dir, spec, true); err != nil {
		t.Fatalf("writeOne force: %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) == "stale" {
		t.Errorf("force did not overwrite")
	}
}

func TestCollectSpecsRejectsBadPlanet(t *testing.T) {
	if _, err := collectSpecs([]string{"badform"}, ""); err == nil {
		t.Errorf("expected error on malformed -planet entry")
	}
}

func TestCollectSpecsManifest(t *testing.T) {
	dir := t.TempDir()
	mpath := filepath.Join(dir, "list.tsv")
	if err := os.WriteFile(mpath, []byte("# comment\nterran\tEarth\nscorched\tMercury\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	specs, err := collectSpecs(nil, mpath)
	if err != nil {
		t.Fatalf("collectSpecs: %v", err)
	}
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(specs))
	}
	// Sorted by seed alphabetically: Earth, Mercury.
	if specs[0].seed != "Earth" || specs[1].seed != "Mercury" {
		t.Errorf("specs not sorted: %+v", specs)
	}
}
