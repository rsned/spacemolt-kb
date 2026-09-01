package wildlife

import (
	"encoding/json"
	"os"
	"slices"
)

// CreatureWeapon is one observed natural weapon: its damage type and the
// range of base (pre-crit, pre-skill) damage seen across exported logs.
type CreatureWeapon struct {
	DamageType string `json:"damage_type"`
	BaseMin    int    `json:"base_min"`
	BaseMax    int    `json:"base_max"`
	Shots      int    `json:"shots"`
}

// CreatureCombat is one species' aggregated combat observations from
// exported battle logs (scripts/wildlife_combat_stats.py).
type CreatureCombat struct {
	Battles   int                       `json:"battles"`
	HullMin   int                       `json:"hull_min"`
	HullMax   int                       `json:"hull_max"`
	ShieldMin int                       `json:"shield_min"`
	ShieldMax int                       `json:"shield_max"`
	HitMin    float64                   `json:"hit_min"`
	HitMax    float64                   `json:"hit_max"`
	Weapons   map[string]CreatureWeapon `json:"weapons"`
}

// DamageTypes lists the distinct damage types this species attacks with,
// sorted for stable rendering.
func (c CreatureCombat) DamageTypes() []string {
	var types []string
	for _, w := range c.Weapons {
		if w.DamageType != "" && !slices.Contains(types, w.DamageType) {
			types = append(types, w.DamageType)
		}
	}
	slices.Sort(types)
	return types
}

// CombatStats is the parsed data/wildlife/combat_stats.json.
type CombatStats struct {
	Source  string                    `json:"source"`
	Species map[string]CreatureCombat `json:"species"`
}

// LoadCombatStats reads a combat_stats.json file.
func LoadCombatStats(path string) (*CombatStats, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cs CombatStats
	if err := json.Unmarshal(raw, &cs); err != nil {
		return nil, err
	}
	return &cs, nil
}
