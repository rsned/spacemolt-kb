package wildlife

import (
	"os"
	"path/filepath"
	"testing"
)

func writeStats(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "battle_stats.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadBattleStats(t *testing.T) {
	path := writeStats(t, `{
  "months": ["2026-07", "2026-08"],
  "species": {
    "Kiln-Snail": {"battles": 7000, "wildlife_wins": 6300},
    "Bell-Jelly": {"battles": 26302, "wildlife_wins": 42},
    "Glacier-Drifter": {"battles": 2, "wildlife_wins": 1}
  }
}`)
	bs, err := LoadBattleStats(path)
	if err != nil {
		t.Fatalf("LoadBattleStats: %v", err)
	}
	if got := len(bs.Species); got != 3 {
		t.Fatalf("species = %d, want 3", got)
	}
	if bs.Months[0] != "2026-07" || bs.Months[1] != "2026-08" {
		t.Errorf("months = %v", bs.Months)
	}

	kiln := bs.Species["Kiln-Snail"]
	if pct := kiln.WinPct(); pct != 90.0 {
		t.Errorf("Kiln-Snail WinPct = %v, want 90", pct)
	}
	if r := kiln.Rating(); r != "extreme" {
		t.Errorf("Kiln-Snail rating = %q, want extreme", r)
	}
	if r := bs.Species["Bell-Jelly"].Rating(); r != "minimal" {
		t.Errorf("Bell-Jelly rating = %q, want minimal", r)
	}
	// Too few battles to rate.
	if r := bs.Species["Glacier-Drifter"].Rating(); r != "" {
		t.Errorf("Glacier-Drifter rating = %q, want unrated", r)
	}
}

func TestBattleRecordRatingTiers(t *testing.T) {
	cases := []struct {
		wins int
		want string
	}{
		{60, "extreme"}, {59, "high"}, {30, "high"}, {29, "moderate"},
		{10, "moderate"}, {9, "low"}, {1, "low"}, {0, "minimal"},
	}
	for _, c := range cases {
		r := BattleRecord{Battles: 100, WildlifeWins: c.wins}
		if got := r.Rating(); got != c.want {
			t.Errorf("%d/100 wins: rating = %q, want %q", c.wins, got, c.want)
		}
	}
}

func TestLoadBattleStatsMissingFile(t *testing.T) {
	if _, err := LoadBattleStats(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Fatal("want error for missing file")
	}
}
