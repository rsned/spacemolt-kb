package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStarRecords(t *testing.T) {
	systems := []*System{
		{ID: "beta", Name: "Beta", PositionX: 10, PositionY: 20,
			POIs: []SystemPOI{{Type: "planet"}, {Type: "sun", Class: "G2V"}}},
		{ID: "alpha", Name: "Alpha", PositionX: 1.5, PositionY: -2.5,
			POIs: []SystemPOI{{Type: "sun", Class: ""}}},
		{ID: "gamma", Name: "Gamma", PositionX: 0, PositionY: 0}, // no sun POI
	}
	recs := starRecords(systems)

	if len(recs) != 3 {
		t.Fatalf("got %d records, want 3", len(recs))
	}
	// Sorted by id for deterministic output.
	if recs[0].ID != "alpha" || recs[1].ID != "beta" || recs[2].ID != "gamma" {
		t.Errorf("not sorted by id: %v", []string{recs[0].ID, recs[1].ID, recs[2].ID})
	}
	// Class comes from the sun POI; blank when the sun has none or there is no sun.
	byID := map[string]StarRecord{}
	for _, r := range recs {
		byID[r.ID] = r
	}
	if byID["beta"].Class != "G2V" {
		t.Errorf("beta class = %q, want G2V", byID["beta"].Class)
	}
	if byID["alpha"].Class != "" {
		t.Errorf("alpha class = %q, want empty (sun has no class)", byID["alpha"].Class)
	}
	if byID["gamma"].Class != "" {
		t.Errorf("gamma class = %q, want empty (no sun POI)", byID["gamma"].Class)
	}
	if byID["beta"].X != 10 || byID["beta"].Y != 20 {
		t.Errorf("beta position = (%v,%v), want (10,20)", byID["beta"].X, byID["beta"].Y)
	}
}

func TestSunClassPrefersBlackHoleAndNonEmpty(t *testing.T) {
	// A black hole is the headline object even when listed after another sun.
	got := sunClass([]SystemPOI{
		{Type: "sun", Class: "K2III"}, {Type: "sun", Class: ""}, {Type: "sun", Class: "BH"},
	})
	if got != "BH" {
		t.Errorf("multi-sun with a black hole = %q, want BH", got)
	}
	// A blank-class sun must not shadow a classified one.
	if got := sunClass([]SystemPOI{{Type: "sun", Class: ""}, {Type: "sun", Class: "K2V"}}); got != "K2V" {
		t.Errorf("prefer non-empty class = %q, want K2V", got)
	}
	// No sun POI -> empty.
	if got := sunClass([]SystemPOI{{Type: "planet", Class: "x"}}); got != "" {
		t.Errorf("no sun = %q, want empty", got)
	}
}

func TestWriteStarsJSONRoundTrip(t *testing.T) {
	systems := []*System{
		{ID: "sol", Name: "Sol", PositionX: 0, PositionY: 0,
			POIs: []SystemPOI{{Type: "sun", Class: "G2V"}}},
	}
	path := filepath.Join(t.TempDir(), "stars.json")
	if err := writeStarsJSON(path, systems); err != nil {
		t.Fatalf("writeStarsJSON: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var got []StarRecord
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if len(got) != 1 || got[0].ID != "sol" || got[0].Class != "G2V" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

func TestWriteStarsJS(t *testing.T) {
	systems := []*System{
		{ID: "sol", Name: "Sol", PositionX: 0, PositionY: 0,
			POIs: []SystemPOI{{Type: "sun", Class: "G2V"}}},
	}
	path := filepath.Join(t.TempDir(), "stars.js")
	if err := writeStarsJS(path, systems); err != nil {
		t.Fatalf("writeStarsJS: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	s := string(data)
	if !strings.HasPrefix(s, "window.SPACEMOLT_STARS=") || !strings.HasSuffix(s, ";") {
		t.Fatalf("not a global-assignment script: %.40q...", s)
	}
	body := strings.TrimSuffix(strings.TrimPrefix(s, "window.SPACEMOLT_STARS="), ";")
	var got []StarRecord
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("embedded array is not valid JSON: %v", err)
	}
	if len(got) != 1 || got[0].ID != "sol" || got[0].Class != "G2V" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}
