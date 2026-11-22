package main

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Mechanics is what a type's raw battle logs show about how it fights:
// resistances, damage reflection, movement and shield behaviour. It is
// derived from the unmodified get_battle_log pages kept in data/battles/
// hostiles/, because the knowledge DB only stores per-weapon totals.
type Mechanics struct {
	Logs  int // battle logs this type appears in
	Ticks int // unit-ticks observed (one per unit per tick)

	Reflect *Reflection // nil when no reflection was seen

	Resists []Resist // by damage type, from the player's hits on it

	Retreats, Advances int // zone moves it chose
	OuterPct           int // share of unit-ticks spent in the outer zone

	MinShieldPct int // lowest shield seen, as % of max; -1 = never seen
	HitsTaken    int // weapon volleys that hit it

	HitByRange []RangeHit // its own hit chance by zone distance

	Destroyed int // units of this type killed
	Kills     int // ships this type destroyed
}

// Reflection is damage bounced back at an attacker: an untargeted, always
// hitting attack whose raw damage is a fixed share of the damage the
// attacker's volley landed on this unit.
type Reflection struct {
	DamageType string
	Pct        int // best-fitting share, floor(landed × Pct / 100)
	Matches    int // reflections the share reproduces exactly
	Samples    int // reflections paired with a landed volley
}

// Resist is a damage-type resistance read off the player's hits.
type Resist struct {
	DamageType string
	Pct        int // most common type_resist_pct
	Hits       int
}

// RangeHit is the mean hit chance at one zone distance (0 = same zone).
type RangeHit struct {
	Distance, Volleys int
	MeanPct           int
}

// rawPage is one get_battle_log page as written by battle-export --raw-out.
type rawPage struct {
	Entries []rawEntry `json:"entries"`
}

type rawEntry struct {
	Tick      int           `json:"tick"`
	Snapshots []rawSnapshot `json:"snapshots"`
	Attacks   []rawAttack   `json:"attacks"`
	ZoneMoves []struct {
		PlayerID string `json:"player_id"`
		Reason   string `json:"reason"`
	} `json:"zone_moves"`
	Kills []struct {
		KillerID string `json:"killer_id"`
		VictimID string `json:"victim_id"`
	} `json:"kills"`
}

type rawSnapshot struct {
	PlayerID  string `json:"player_id"`
	ShipClass string `json:"ship_class"`
	Zone      string `json:"zone"`
	Shield    int    `json:"shield"`
	MaxShield int    `json:"max_shield"`
}

type rawAttack struct {
	AttackerID    string            `json:"attacker_id"`
	TargetID      string            `json:"target_id"`
	HitChance     float64           `json:"hit_chance"`
	HitSuccess    bool              `json:"hit_success"`
	DamageType    string            `json:"damage_type"`
	ZoneDistance  int               `json:"zone_distance"`
	Splash        bool              `json:"splash"`
	RawDamage     int               `json:"raw_damage"`
	LandedDamage  int               `json:"landed_damage"`
	TypeResistPct int               `json:"type_resist_pct"`
	Weapons       []json.RawMessage `json:"weapons"`
}

// hostileTypeOf maps a battle participant to its hostile type id, or "" when
// it is not a hostile (players, wildlife). Repeater ids look like
// repeater_<hex>; the type is repeater_<ship class>.
func hostileTypeOf(playerID, shipClass string) string {
	if strings.HasPrefix(playerID, "repeater_") && shipClass != "" {
		return "repeater_" + shipClass
	}
	return ""
}

// mechAcc accumulates one type's observations across logs.
type mechAcc struct {
	logs, ticks, retreats, advances, outer, destroyed, kills, hitsTaken int
	minShield                                                           float64
	shieldSeen                                                          bool
	reflType                                                            string
	refl                                                                [][2]int // {raw reflected, landed}
	resist                                                              map[string]map[int]int
	hit                                                                 map[int][]float64
}

// loadMechanics reads every *.raw.json log in dir. A missing dir is not an
// error: the pages then show only what the knowledge DB holds.
func loadMechanics(dir string) (map[string]*Mechanics, int, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.raw.json"))
	if err != nil {
		return nil, 0, err
	}
	acc := map[string]*mechAcc{}
	get := func(t string) *mechAcc {
		a := acc[t]
		if a == nil {
			a = &mechAcc{minShield: 1, resist: map[string]map[int]int{}, hit: map[int][]float64{}}
			acc[t] = a
		}
		return a
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, 0, err
		}
		var pages []rawPage
		if err := json.Unmarshal(data, &pages); err != nil {
			return nil, 0, fmt.Errorf("%s: %w", f, err)
		}
		var entries []rawEntry
		for _, p := range pages {
			entries = append(entries, p.Entries...)
		}
		addLog(entries, get)
	}
	out := map[string]*Mechanics{}
	for t, a := range acc {
		out[t] = a.summarize()
	}
	return out, len(files), nil
}

// addLog folds one battle's entries into the per-type accumulators.
func addLog(entries []rawEntry, get func(string) *mechAcc) {
	typeOf := map[string]string{} // participant id → hostile type
	for _, e := range entries {
		for _, s := range e.Snapshots {
			if t := hostileTypeOf(s.PlayerID, s.ShipClass); t != "" {
				typeOf[s.PlayerID] = t
			}
		}
	}
	seen := map[string]bool{}
	for _, t := range typeOf {
		if !seen[t] {
			seen[t] = true
			get(t).logs++
		}
	}
	for _, e := range entries {
		for _, s := range e.Snapshots {
			t := typeOf[s.PlayerID]
			if t == "" {
				continue
			}
			a := get(t)
			a.ticks++
			if s.Zone == "outer" {
				a.outer++
			}
			if s.MaxShield > 0 {
				a.shieldSeen = true
				a.minShield = min(a.minShield, float64(s.Shield)/float64(s.MaxShield))
			}
		}
		for _, z := range e.ZoneMoves {
			t := typeOf[z.PlayerID]
			if t == "" {
				continue
			}
			switch z.Reason {
			case "retreat":
				get(t).retreats++
			case "advance":
				get(t).advances++
			}
		}
		for _, k := range e.Kills {
			if t := typeOf[k.VictimID]; t != "" {
				get(t).destroyed++
			}
			if t := typeOf[k.KillerID]; t != "" && typeOf[k.VictimID] == "" {
				get(t).kills++
			}
		}
		for _, at := range e.Attacks {
			attacker, target := typeOf[at.AttackerID], typeOf[at.TargetID]
			switch {
			case attacker != "" && target == "" && len(at.Weapons) == 0 && !at.Splash:
				// A weaponless attack is a reflection; pair it with what the
				// target's own volleys landed on this unit this tick.
				landed := 0
				for _, v := range e.Attacks {
					if v.AttackerID == at.TargetID && v.TargetID == at.AttackerID && len(v.Weapons) > 0 {
						landed += v.LandedDamage
					}
				}
				a := get(attacker)
				a.reflType = at.DamageType
				if landed > 0 {
					a.refl = append(a.refl, [2]int{at.RawDamage, landed})
				}
			case attacker != "" && target == "":
				a := get(attacker)
				a.hit[at.ZoneDistance] = append(a.hit[at.ZoneDistance], at.HitChance)
			case target != "" && attacker == "" && at.HitSuccess && !at.Splash && len(at.Weapons) > 0:
				a := get(target)
				a.hitsTaken++
				if a.resist[at.DamageType] == nil {
					a.resist[at.DamageType] = map[int]int{}
				}
				a.resist[at.DamageType][at.TypeResistPct]++
			}
		}
	}
}

func (a *mechAcc) summarize() *Mechanics {
	m := &Mechanics{Logs: a.logs, Ticks: a.ticks, Retreats: a.retreats, Advances: a.advances,
		Destroyed: a.destroyed, Kills: a.kills, HitsTaken: a.hitsTaken, MinShieldPct: -1}
	if a.ticks > 0 {
		m.OuterPct = 100 * a.outer / a.ticks
	}
	if a.shieldSeen {
		m.MinShieldPct = int(a.minShield * 100)
	}
	if a.reflType != "" {
		m.Reflect = fitReflection(a.reflType, a.refl)
	}
	for dt, byPct := range a.resist {
		r := Resist{DamageType: dt}
		best := -1
		for pct, n := range byPct {
			r.Hits += n
			if n > best || (n == best && pct < r.Pct) {
				best, r.Pct = n, pct
			}
		}
		m.Resists = append(m.Resists, r)
	}
	slices.SortFunc(m.Resists, func(x, y Resist) int { return cmp.Compare(x.DamageType, y.DamageType) })
	for d, hs := range a.hit {
		sum := 0.0
		for _, h := range hs {
			sum += h
		}
		m.HitByRange = append(m.HitByRange, RangeHit{Distance: d, Volleys: len(hs), MeanPct: int(100*sum/float64(len(hs)) + 0.5)})
	}
	slices.SortFunc(m.HitByRange, func(x, y RangeHit) int { return cmp.Compare(x.Distance, y.Distance) })
	return m
}

// fitReflection finds the whole-percent share that reproduces the most
// reflections exactly as floor(landed × pct / 100).
func fitReflection(dt string, pairs [][2]int) *Reflection {
	r := &Reflection{DamageType: dt, Samples: len(pairs)}
	for pct := 1; pct <= 100; pct++ {
		n := 0
		for _, p := range pairs {
			if p[0] == p[1]*pct/100 {
				n++
			}
		}
		if n > r.Matches {
			r.Pct, r.Matches = pct, n
		}
	}
	return r
}

// MatchPct is the share of reflections the fitted percentage reproduces.
func (r Reflection) MatchPct() int {
	if r.Samples == 0 {
		return 0
	}
	return 100 * r.Matches / r.Samples
}
