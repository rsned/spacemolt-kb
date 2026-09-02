package combatsim

import (
	"encoding/json"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
)

// battleFixture is the subset of an exported battle JSON (data/battles/*.json,
// bin/battle-export's output) this validation test needs: who won and how
// long the fight took.
type battleFixture struct {
	WinningSide int `json:"winning_side"`
	TickCount   int `json:"tick_count"`
}

func loadBattleFixture(t *testing.T, id string) battleFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "data", "battles", id+".json"))
	if err != nil {
		t.Fatalf("read fixture %s: %v", id, err)
	}
	var f battleFixture
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("parse fixture %s: %v", id, err)
	}
	return f
}

// validationCase maps one committed multi-participant fixture down to the
// subset of participants that actually fought, using the hand-built
// StatBlocks already characterized in engine_test.go (or, for 7c044558,
// duplicated locally from TestGoldenVolleys' inline blocks — those are
// declared inside that test func, not package-level, so aren't directly
// reusable). sideTeam maps the fixture's side_id to the Team index used in
// ships.
type validationCase struct {
	name     string
	fixture  string // battle id (filename stem under data/battles/)
	ships    []Ship
	sideTeam map[int]int // fixture side_id -> Ship.Team
	excluded string      // t.Log note: participants dropped and why
}

func validationCases() []validationCase {
	// 7c044558: Artis's survey_vessel vs MoltenOne's broadaxe, both fully
	// combat-capable (no participants excluded). Stat blocks mirror
	// TestGoldenVolleys' inline "7c04" cases exactly (same source data).
	moltenBroadaxe := &StatBlock{Name: "moltenBx", WeaponSkillPct: 17, CritPct: 7,
		MaxHull: 282, MaxShield: 28, Recharge: 2, ArmorTotal: 28, ShieldsSkill: 4,
		Weapons: []Weapon{{Damage: 14, Type: "kinetic"}, {Damage: 14, Type: "kinetic"}, {Damage: 14, Type: "kinetic"}, {Damage: 28, Type: "kinetic"}}}
	artisSurvey := &StatBlock{Name: "artisSurvey", WeaponSkillPct: 6, CritPct: 3,
		MaxHull: 340, MaxShield: 400, Recharge: 9, ArmorTotal: 14, ShieldsSkill: 1,
		Weapons: []Weapon{{Damage: 28, Type: "energy"}, {Damage: 28, Type: "energy"}}}

	return []validationCase{
		{
			name:    "509e Artis vs MoltenOne",
			fixture: "509e1ef4a76fc90d7ce4e33c85336a68",
			ships: []Ship{
				{Stats: artisEviction(), Team: 0}, // side 1: Arthur 'Artificer' Artis, eviction_notice
				{Stats: molten509(), Team: 1},     // side 2: MoltenOne, portfolio
			},
			sideTeam: map[int]int{1: 0, 2: 1},
			excluded: "side 1's Preston 'Profit' Price (bedrock, Mining Laser II only) carried no weapon module and was never the volley target (MoltenOne always targets the first living enemy, Artis) — dropped, no hand-built StatBlock needed.",
		},
		{
			name:    "b7847bbc MoltenOne vs Vera",
			fixture: "b7847bbc62a59f67f503ab3c65fb0897",
			ships: []Ship{
				{Stats: moltenUnderwriter(), Team: 0}, // side 1: MoltenOne, underwriter
				{Stats: vera(), Team: 1},              // side 2: VeraLane_Zibal, cobble
			},
			sideTeam: map[int]int{1: 0, 2: 1},
			excluded: "side 2's Mining Drone 005/006 (lithosphere, Mining Laser V only, no weapon module) left the battle (last_tick one before the fixture's final tick, no destroyed_at_tick) rather than being destroyed — RunMultiShip has no flee model for multi-ship battles (every Ship fires from a fixed StanceFire), so they can't be reproduced; dropped as combat-inert.",
		},
		{
			name:    "7c044558 Artis vs MoltenOne",
			fixture: "7c044558c0c39e972fe560110f69ea25",
			ships: []Ship{
				{Stats: artisSurvey, Team: 0},   // side 1: Arthur 'Artificer' Artis, survey_vessel
				{Stats: moltenBroadaxe, Team: 1}, // side 2: MoltenOne, broadaxe
			},
			sideTeam: map[int]int{1: 0, 2: 1},
			excluded: "none — both participants are combat-capable and already characterized.",
		},
	}
}

// TestValidateAgainstRealBattles replays committed real multi-participant
// battle fixtures (data/battles/*.json) through RunMultiShip and checks the
// model's predicted majority winner and typical duration against what
// actually happened, using the hand-built StatBlocks already verified
// per-volley against the same logs in TestGoldenVolleys. The engine targets
// population-level outcomes, not exact turn-by-turn replays, so the duration
// band is generous and any fixture whose participants can't be resolved to
// StatBlocks is skipped rather than forced.
func TestValidateAgainstRealBattles(t *testing.T) {
	const runs = 200
	cal := DefaultCalibration()

	for _, tc := range validationCases() {
		t.Run(tc.name, func(t *testing.T) {
			if tc.excluded != "" {
				t.Log("excluded participants:", tc.excluded)
			}
			fx := loadBattleFixture(t, tc.fixture)
			wantTeam, ok := tc.sideTeam[fx.WinningSide]
			if !ok {
				t.Fatalf("fixture %s: winning_side %d has no team mapping", tc.fixture, fx.WinningSide)
			}

			wins := map[int]int{}
			ticks := make([]int, 0, runs)
			for s := range uint64(runs) {
				rng := randForRun(s, tc.fixture)
				r := RunMultiShip(tc.ships, cal, cal.MaxTicks, rng)
				wins[r.WinningTeam]++
				ticks = append(ticks, r.Ticks)
			}

			if wins[wantTeam] <= runs/2 {
				t.Errorf("fixture %s: team %d (real winner) won %d/%d runs, want majority; tally=%v",
					tc.fixture, wantTeam, wins[wantTeam], runs, wins)
			}
			gotTicks := median(ticks)
			lo, hi := float64(fx.TickCount)*0.3, float64(fx.TickCount)*3
			if float64(gotTicks) < lo || float64(gotTicks) > hi {
				t.Errorf("fixture %s: median ticks %d outside [%.1f, %.1f] band around real tick_count %d",
					tc.fixture, gotTicks, lo, hi, fx.TickCount)
			}
			t.Logf("fixture %s: winner team %d %d/%d runs (real side %d), median ticks %d (real %d)",
				tc.fixture, wantTeam, wins[wantTeam], runs, fx.WinningSide, gotTicks, fx.TickCount)
		})
	}

	for _, sk := range skippedFixtures() {
		t.Run(sk.name, func(t *testing.T) {
			t.Skip(sk.reason)
		})
	}
}

// skippedCase records a committed fixture that this test deliberately does
// not validate against, and why.
type skippedCase struct {
	name, reason string
}

// skippedFixtures lists the remaining committed data/battles/*.json fixtures
// this task did not resolve to StatBlocks: all involve pirate/police/station
// NPCs (no hand-built or catalog-resolvable stat blocks — the exported
// participant "modules" carry only display names, not damage/type/slot) at
// N too large (5, 13, 42 participants) to hand-characterize within this
// task's scope. Recorded here so the fixture inventory is visible in test
// output rather than silently unexercised.
func skippedFixtures() []skippedCase {
	return []skippedCase{
		{"b131fd5aae68420107dd20e93d15d3ba (5 participants, pirates+players, 4 sides)",
			"pirate NPC (lemming/red_herring) stat blocks not hand-characterized; module names not resolvable to StatBlocks without full catalog-driven resolution"},
		{"2a76e1a1c796e9d8877fdeedb76867ec (13 participants, station + 10 players)",
			"station combat participant and 10 unhand-characterized player fits; well beyond this task's small-N hand-built scope"},
		{"a2619bbe328676445828b4e1007fe9aa (42 participants, 11 pirates vs 31 police/station)",
			"pirate and police NPC classes (anamnesis/annihilation/paradox/etc.) have no hand-built or catalog-resolvable StatBlocks"},
	}
}

// randForRun mirrors the rand.New(rand.NewPCG(s+1, tag)) seeding pattern
// used throughout this package's Monte-Carlo tests (see swarm_test.go),
// hashing the fixture id into the second seed word so different cases don't
// share a stream.
func randForRun(s uint64, tag string) *rand.Rand {
	var h uint64 = 14695981039346656037 // FNV-1a offset basis
	for i := range len(tag) {
		h ^= uint64(tag[i])
		h *= 1099511628211
	}
	return rand.New(rand.NewPCG(s+1, h))
}
