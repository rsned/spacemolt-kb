// Command combat-sim Monte-Carlo simulates 1v1 combat between two ship
// fittings using the battle-log-verified damage model. Hermetic: reads only
// committed catalog snapshots and small JSON input files.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type ShipDef struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	Class              string   `json:"class"`
	Tier               int      `json:"tier"`
	BaseHull           int      `json:"base_hull"`
	BaseShield         int      `json:"base_shield"`
	BaseShieldRecharge int      `json:"base_shield_recharge"`
	BaseArmor          int      `json:"base_armor"`
	DefaultModules     []string `json:"default_modules"`
}

type ItemDef struct {
	ID                  string         `json:"id"`
	Name                string         `json:"name"`
	Type                string         `json:"type"`
	Slot                string         `json:"slot"`
	Damage              int            `json:"damage"`
	DamageType          string         `json:"damage_type"`
	Reach               int            `json:"reach"`
	Cooldown            int            `json:"cooldown"`
	MagazineSize        int            `json:"magazine_size"`
	ShieldBonus         int            `json:"shield_bonus"`
	ArmorBonus          int            `json:"armor_bonus"`
	DamageReduction     int            `json:"damage_reduction"`
	ShieldRechargeBonus int            `json:"shield_recharge_bonus"`
	RequiredSkills      map[string]int `json:"required_skills"`
}

type Catalog struct {
	Ships map[string]*ShipDef
	Items map[string]*ItemDef
}

func loadItems[T any](path string) ([]T, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Items []T `json:"items"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return wrap.Items, nil
}

// LoadCatalog reads catalog_ships.json and catalog_items.json from dir.
func LoadCatalog(dir string) (*Catalog, error) {
	ships, err := loadItems[*ShipDef](filepath.Join(dir, "catalog_ships.json"))
	if err != nil {
		return nil, err
	}
	items, err := loadItems[*ItemDef](filepath.Join(dir, "catalog_items.json"))
	if err != nil {
		return nil, err
	}
	cat := &Catalog{Ships: map[string]*ShipDef{}, Items: map[string]*ItemDef{}}
	for _, s := range ships {
		cat.Ships[s.ID] = s
	}
	for _, it := range items {
		cat.Items[it.ID] = it
	}
	return cat, nil
}
