package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestEmpireColor(t *testing.T) {
	if got := empireColor("crimson"); got != "#DC143C" {
		t.Fatalf("crimson = %q, want #DC143C", got)
	}
	if got := empireColor("NEBULA"); got != "#00CED1" {
		t.Fatalf("NEBULA = %q, want #00CED1 (case-insensitive)", got)
	}
	if got := empireColor("unknown"); got != "" {
		t.Fatalf("unknown empire = %q, want empty", got)
	}
}

func TestLoadPassengers(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	_, err = db.Exec(`CREATE TABLE passengers (
		citizen_id TEXT PRIMARY KEY, name TEXT NOT NULL, citizenship TEXT,
		bio TEXT, class TEXT, first_seen_utc TEXT NOT NULL,
		last_seen_utc TEXT NOT NULL, sighting_count INTEGER NOT NULL DEFAULT 1)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO passengers VALUES
		('b_id','Bea','nebula','rich bio','business','2026-01-01T00:00:00Z','2026-01-02T00:00:00Z',3),
		('a_id','Abe','crimson','','first','2026-01-01T00:00:00Z','2026-01-03T00:00:00Z',1)`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := loadPassengers(db)
	if err != nil {
		t.Fatalf("loadPassengers: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d passengers, want 2", len(got))
	}
	if got[0].Name != "Abe" || got[1].Name != "Bea" {
		t.Fatalf("unexpected order: %s, %s", got[0].Name, got[1].Name)
	}
	if got[0].EmpireColor != "#DC143C" {
		t.Fatalf("Abe empire color = %q, want #DC143C", got[0].EmpireColor)
	}
	if got[1].SightingCount != 3 || got[1].Class != "business" {
		t.Fatalf("Bea fields wrong: count=%d class=%s", got[1].SightingCount, got[1].Class)
	}
	if got[0].Slug != "a_id" {
		t.Fatalf("Slug should equal citizen_id, got %q", got[0].Slug)
	}
}

func TestAttachPassengerPortraitsMissingFileIsSilent(t *testing.T) {
	root := t.TempDir()
	ps := []*Passenger{{ID: "nobody", Slug: "nobody"}}
	attachPassengerPortraits(ps, root) // no file on disk
	if ps[0].PortraitFile != "" {
		t.Fatalf("expected empty PortraitFile, got %q", ps[0].PortraitFile)
	}
}

func TestAttachPassengerPortraitsValidImage(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "generated", "passengers", "p1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writePNG(t, filepath.Join(dir, generatedPortraitName), 1, 1)
	ps := []*Passenger{{ID: "p1", Slug: "p1"}}
	attachPassengerPortraits(ps, root)
	if ps[0].PortraitFile != generatedPortraitName {
		t.Fatalf("PortraitFile = %q, want %q", ps[0].PortraitFile, generatedPortraitName)
	}
}

func TestAttachPassengerOverlays(t *testing.T) {
	root := t.TempDir()
	// Overlay for passenger "abc"; an orphan dir "ghost" with no matching passenger.
	mk := func(id, profile string) {
		d := filepath.Join(root, "passengers", id)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "profile.md"), []byte(profile), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("abc", "body for abc")
	mk("ghost", "orphan body")

	passengers := []*Passenger{{ID: "abc"}, {ID: "xyz"}}
	attachPassengerOverlays(passengers, root)
	if passengers[0].Overlay == nil {
		t.Fatal("passenger abc should have an overlay")
	}
	if passengers[0].Overlay.BodyHTML == "" {
		t.Errorf("passenger abc overlay body should be populated: %+v", passengers[0].Overlay)
	}
	if passengers[1].Overlay != nil {
		t.Error("passenger xyz should have no overlay")
	}
}
