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
	writeTinyPNG(t, filepath.Join(dir, generatedPortraitName))
	ps := []*Passenger{{ID: "p1", Slug: "p1"}}
	attachPassengerPortraits(ps, root)
	if ps[0].PortraitFile != generatedPortraitName {
		t.Fatalf("PortraitFile = %q, want %q", ps[0].PortraitFile, generatedPortraitName)
	}
}

func writeTinyPNG(t *testing.T, path string) {
	t.Helper()
	// 1x1 transparent PNG.
	png := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
		0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00,
		0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49,
		0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
	}
	if err := os.WriteFile(path, png, 0o644); err != nil {
		t.Fatal(err)
	}
}
