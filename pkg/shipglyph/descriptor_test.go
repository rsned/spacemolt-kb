package shipglyph

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMergeOverlayWinsFieldByField(t *testing.T) {
	base := Descriptor{
		ID:      "prayer",
		Aspect:  2.0,
		Greeble: "light",
		Hull:    []HullPart{{Kind: "beam", Span: [2]float64{0, 1}}},
		MountZones: MountZones{
			Weapon: [][2]float64{{0.1, 0.4}},
		},
	}
	over := Descriptor{
		Aspect: 3.2,
		Hull: []HullPart{
			{Kind: "container_stack", Span: [2]float64{0.15, 0.75}, Grid: [2]int{2, 2}},
		},
	}

	got := Merge(base, over)

	if got.Aspect != 3.2 {
		t.Errorf("Aspect = %v, want 3.2 (overlay wins)", got.Aspect)
	}
	if got.Greeble != "light" {
		t.Errorf("Greeble = %q, want %q (base survives)", got.Greeble, "light")
	}
	if len(got.Hull) != 1 || got.Hull[0].Kind != "container_stack" {
		t.Errorf("Hull = %+v, want the overlay's container_stack", got.Hull)
	}
	if len(got.MountZones.Weapon) != 1 {
		t.Errorf("MountZones.Weapon = %+v, want base's zone to survive", got.MountZones.Weapon)
	}
	if got.ID != "prayer" {
		t.Errorf("ID = %q, want %q", got.ID, "prayer")
	}
}

func TestLoadOverlayMissingFileIsNotAnError(t *testing.T) {
	_, ok, err := LoadOverlay(t.TempDir(), "nope")
	if err != nil {
		t.Fatalf("LoadOverlay returned error for missing file: %v", err)
	}
	if ok {
		t.Errorf("ok = true, want false for a missing overlay")
	}
}

func TestLoadOverlayParsesJSON(t *testing.T) {
	dir := t.TempDir()
	body := `{
	  "id": "prayer",
	  "aspect": 3.2,
	  "symmetry": "bilateral",
	  "hull": [
	    {"kind": "container_stack", "span": [0.15, 0.75], "grid": [2, 2]},
	    {"kind": "engine_cone", "span": [0.75, 1.0], "bells": 4}
	  ],
	  "appendages": [{"kind": "wing", "at": 0.62, "sweep": 38, "span": 0.55, "side": "both"}],
	  "mountZones": {"weapon": [[0.1, 0.45]]},
	  "greeble": "heavy"
	}`
	if err := os.WriteFile(filepath.Join(dir, "prayer.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	d, ok, err := LoadOverlay(dir, "prayer")
	if err != nil || !ok {
		t.Fatalf("LoadOverlay: ok=%v err=%v", ok, err)
	}
	if len(d.Hull) != 2 {
		t.Fatalf("Hull len = %d, want 2", len(d.Hull))
	}
	if d.Hull[0].Grid != [2]int{2, 2} {
		t.Errorf("Grid = %v, want [2 2]", d.Hull[0].Grid)
	}
	if d.Hull[1].Bells != 4 {
		t.Errorf("Bells = %d, want 4", d.Hull[1].Bells)
	}
	if len(d.Appendages) != 1 || d.Appendages[0].Sweep != 38 {
		t.Errorf("Appendages = %+v", d.Appendages)
	}
	if d.Greeble != "heavy" {
		t.Errorf("Greeble = %q, want heavy", d.Greeble)
	}
}
