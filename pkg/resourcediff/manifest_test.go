package resourcediff

import "testing"

func TestManifestLookups(t *testing.T) {
	m := &Manifest{Snapshots: []ManifestEntry{
		{Date: "2026-08-27", Baseline: true, ContentVersion: "0.566.0"},
		{Date: "2026-08-29"},
		{Date: "2026-09-02", Baseline: true, ContentVersion: "0.570.0"},
		{Date: "2026-09-05"},
	}}
	if p := m.PreviousBefore("2026-09-05"); p == nil || p.Date != "2026-09-02" {
		t.Errorf("PreviousBefore(09-05) = %+v", p)
	}
	if p := m.PreviousBefore("2026-08-27"); p != nil {
		t.Errorf("PreviousBefore(first) = %+v", p)
	}
	if b := m.BaselineFor("2026-08-29"); b == nil || b.ContentVersion != "0.566.0" {
		t.Errorf("BaselineFor(08-29) = %+v", b)
	}
	if b := m.BaselineFor("2026-09-05"); b == nil || b.ContentVersion != "0.570.0" {
		t.Errorf("BaselineFor(09-05) = %+v", b)
	}
	if b := m.BaselineFor("2026-09-02"); b == nil || b.Date != "2026-09-02" {
		t.Errorf("BaselineFor(baseline day) = %+v", b)
	}
	if b := m.BaselineFor("2026-01-01"); b != nil {
		t.Errorf("BaselineFor(before any) = %+v", b)
	}
	dir := t.TempDir()
	if err := m.Save(dir); err != nil {
		t.Fatal(err)
	}
	back, err := LoadManifest(dir)
	if err != nil || len(back.Snapshots) != 4 {
		t.Fatalf("round trip: %v %+v", err, back)
	}
	if empty, err := LoadManifest(t.TempDir()); err != nil || len(empty.Snapshots) != 0 {
		t.Errorf("missing manifest should be empty: %v %+v", err, empty)
	}
}
