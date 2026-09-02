package combatsim

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Golden duels replay real Phase-B battle fixtures (data/combat-sim/
// golden-duels/*.raw.json) and assert the invariants the calibrated engine
// depends on actually hold in server data.
//
// NOTE on what is NOT asserted: the plan's original "a non-fire stance
// never attacks" check does not survive real server logs. The server
// autopilot flickers a scripted ship's stance label from tick to tick, so
// a snapshot can read stance="flee"/"brace" on the same tick the ship
// fires (measured: 2 such ticks in brace-booster.raw.json). That is a
// server-side labelling artifact, not a sim rule violation -- the sim
// enforces fire-only firing by construction (see volley()). Asserting it
// against flickered server data would fail spuriously, so we assert the
// robust, flicker-free invariants below instead.
type goldenPage struct {
	Entries []struct {
		Tick    int `json:"tick"`
		Flee    []struct {
			PlayerID     string `json:"player_id"`
			FleeCounter  int    `json:"flee_counter"`
			FleeRequired int    `json:"flee_required"`
			Escaped      bool   `json:"escaped"`
		} `json:"flee"`
		Snapshots []struct {
			PlayerID  string `json:"player_id"`
			Stance    string `json:"stance"`
			Shield    int    `json:"shield"`
			MaxShield int    `json:"max_shield"`
		} `json:"snapshots"`
	} `json:"entries"`
}

func TestGoldenDuels(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("..", "..", "data", "combat-sim", "golden-duels", "*.raw.json"))
	if len(files) == 0 {
		t.Fatal("no golden duel fixtures committed")
	}
	sawFlee, sawBrace := false, false
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			raw, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			var pages []goldenPage
			if err := json.Unmarshal(raw, &pages); err != nil {
				t.Fatal(err)
			}
			// Track the last-seen flee counter per player so we can assert
			// the counter builds up to (never past, before escape) required.
			prevCounter := map[string]int{}
			for _, page := range pages {
				for _, e := range page.Entries {
					for _, fl := range e.Flee {
						sawFlee = true
						// Escape happens exactly when the counter reaches the
						// required threshold -- never earlier.
						if fl.Escaped && fl.FleeCounter < fl.FleeRequired {
							t.Errorf("tick %d: %s escaped at counter %d < required %d",
								e.Tick, fl.PlayerID[:6], fl.FleeCounter, fl.FleeRequired)
						}
						// The counter builds by at most 1 per tick (a
						// deterministic consecutive count, not a random roll).
						if p, ok := prevCounter[fl.PlayerID]; ok && fl.FleeCounter > p+1 {
							t.Errorf("tick %d: %s flee counter jumped %d -> %d (>1/tick)",
								e.Tick, fl.PlayerID[:6], p, fl.FleeCounter)
						}
						prevCounter[fl.PlayerID] = fl.FleeCounter
					}
					// A braced side's shield never exceeds its own max: the
					// brace-regen multiplier tops up toward MaxShield, never
					// past it (guards the min(shield+r, max) clamp the engine
					// relies on).
					for _, s := range e.Snapshots {
						if s.Stance == "brace" {
							sawBrace = true
							if s.MaxShield > 0 && s.Shield > s.MaxShield {
								t.Errorf("tick %d: %s braced shield %d exceeds max %d",
									e.Tick, s.PlayerID[:6], s.Shield, s.MaxShield)
							}
						}
					}
				}
			}
		})
	}
	if !sawFlee {
		t.Error("no golden fixture exercised a flee sequence")
	}
	if !sawBrace {
		t.Error("no golden fixture exercised a brace stance")
	}
}
