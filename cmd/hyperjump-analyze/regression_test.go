package main

import (
	"database/sql"
	"os"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/hyperjump"

	_ "modernc.org/sqlite"
)

// realDB returns an open handle to the live KB if it can be found, else skips.
func realDB(t *testing.T) *sql.DB {
	t.Helper()
	candidates := []string{
		"../../../spacemolt/data/spacemolt-knowledge.db",
		"../../../spacemolt-knowledge.db",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			db, err := sql.Open("sqlite", p)
			if err != nil {
				t.Fatalf("open %s: %v", p, err)
			}
			return db
		}
	}
	t.Skip("live KB not found; skipping regression test")
	return nil
}

// TestSolEscapeDirections locks the end-to-end pipeline against the live galaxy:
// from Sol, headings are 99.32% blocked, leaving 6 narrow void gaps; the widest
// is ~1.17 deg centered near heading 15.5 deg.
func TestSolEscapeDirections(t *testing.T) {
	db := realDB(t)
	defer func() { _ = db.Close() }()

	systems, err := loadSystems(db)
	if err != nil {
		t.Fatalf("loadSystems: %v", err)
	}

	var sol hyperjump.System
	found := false
	for _, s := range systems {
		if s.ID == "sol" {
			sol, found = s, true
			break
		}
	}
	if !found {
		t.Skip("sol not in KB; skipping")
	}

	pct, gaps := hyperjump.Coverage(sol, systems, 100)

	if pct < 0.99 || pct > 0.995 {
		t.Errorf("Sol coverage = %.5f, want ~0.9932", pct)
	}
	if len(gaps) != 6 {
		t.Fatalf("Sol gaps = %d, want 6", len(gaps))
	}
	widest := gaps[0] // Coverage returns gaps widest-first
	if widest.WidthDeg < 1.0 || widest.WidthDeg > 1.3 {
		t.Errorf("widest gap = %.4f deg, want ~1.17", widest.WidthDeg)
	}
	if widest.CenterDeg < 15.0 || widest.CenterDeg > 16.0 {
		t.Errorf("widest gap center = %.4f deg, want ~15.5", widest.CenterDeg)
	}
}
