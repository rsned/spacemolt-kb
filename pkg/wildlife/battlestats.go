package wildlife

import (
	"encoding/json"
	"os"
)

// minRatedBattles is the sample-size floor below which a win rate is too
// noisy to summarize as a danger tier (a 1-for-2 record is not "extreme").
const minRatedBattles = 25

// BattleRecord is one species' record across the bulk feed's wildlife
// battles: how many it appeared in, and how many the wildlife side won
// outright. Draws (stalemate, mutual destruction) count for neither side.
type BattleRecord struct {
	Battles      int `json:"battles"`
	WildlifeWins int `json:"wildlife_wins"`
}

// WinPct is the wildlife side's win rate as a percentage.
func (r BattleRecord) WinPct() float64 {
	if r.Battles == 0 {
		return 0
	}
	return 100 * float64(r.WildlifeWins) / float64(r.Battles)
}

// Rating buckets the win rate into a danger tier, or "" when the sample is
// too small to rate.
func (r BattleRecord) Rating() string {
	if r.Battles < minRatedBattles {
		return ""
	}
	switch pct := r.WinPct(); {
	case pct >= 60:
		return "extreme"
	case pct >= 30:
		return "high"
	case pct >= 10:
		return "moderate"
	case pct >= 1:
		return "low"
	default:
		return "minimal"
	}
}

// BattleStats is the parsed data/wildlife/battle_stats.json, produced by
// scripts/wildlife_battle_stats.py from the public bulk data feed's monthly
// battle shards. Species keys are display names (wildlife_species.name).
type BattleStats struct {
	Months  []string                `json:"months"`
	Species map[string]BattleRecord `json:"species"`
}

// LoadBattleStats reads a battle_stats.json file.
func LoadBattleStats(path string) (*BattleStats, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var bs BattleStats
	if err := json.Unmarshal(raw, &bs); err != nil {
		return nil, err
	}
	return &bs, nil
}
