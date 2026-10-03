package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/tectonics"
)

func TestRunBundleWritesManifestAndFrames(t *testing.T) {
	out := t.TempDir()
	var last int
	dir, err := runBundle(runOpts{Seed: 5, Archetype: "arid", Out: out, Face: 16, Steps: 3,
		Sets: []string{"KeyframeEvery=1", "RidgeThickness=0.2"}, Progress: func(s, _ int) { last = s }})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(dir) != "seed-5-arid" {
		t.Errorf("dir %s", dir)
	}
	m, err := tectonics.ReadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Frames) != 4 || m.Params.RidgeThickness != 0.2 || m.Face != 16 || m.Seed != 5 || last != 3 {
		t.Errorf("manifest %+v last=%d", m, last)
	}
	if _, err := tectonics.ReadFrame(dir, m.Frames[3]); err != nil {
		t.Fatal(err)
	}
}

func TestRunBundleRefusesGasGiantAndBadSet(t *testing.T) {
	if _, err := runBundle(runOpts{Seed: 1, Archetype: "jovian", Out: t.TempDir(), Face: 16, Steps: 1}); err == nil {
		t.Error("jovian must be refused")
	}
	if _, err := runBundle(runOpts{Seed: 1, Archetype: "arid", Out: t.TempDir(), Face: 16, Steps: 1, Sets: []string{"Nope=1"}}); err == nil {
		t.Error("unknown knob must be refused")
	}
	if _, err := runBundle(runOpts{Planet: "x_i", Archetype: "arid", Out: t.TempDir(), Face: 16, Steps: 1, Sets: []string{"garbage"}}); err == nil {
		t.Error("malformed -set must be refused")
	}
}

func TestRunBundleRejectsTraversalPlanetIDs(t *testing.T) {
	for _, id := range []string{"../../x", "a/b", "a..b"} {
		out := t.TempDir()
		if _, err := runBundle(runOpts{Planet: id, Archetype: "arid", Out: out, Face: 16, Steps: 1}); err == nil {
			t.Errorf("planet id %q must be refused", id)
		}
		entries, err := os.ReadDir(out)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Errorf("planet id %q: Out dir not empty: %v", id, entries)
		}
	}
}
