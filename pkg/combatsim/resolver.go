package combatsim

import (
	"encoding/json"
	"fmt"
	"os"
)

type FitSpec struct {
	Name    string         `json:"name"`
	Hull    string         `json:"hull"`
	Modules []string       `json:"modules"`
	Skills  map[string]int `json:"skills"` // keys: weapons, gunnery, shields, armor (missing = 0)
}

type Weapon struct {
	Name     string
	Damage   int
	Type     string // energy|kinetic|void|explosive|em|thermal
	Cooldown int
	Magazine int // 0 = no ammo tracking (beam weapons)
	Reach    int
}

type StatBlock struct {
	Name           string
	MaxHull        int
	MaxShield      int
	Recharge       int
	ArmorTotal     float64 // (base_armor + Σ armor_bonus) × (1 + Armor×0.01)
	FlatPct        int     // Σ damage_reduction, capped 75
	ShieldsSkill   int
	WeaponSkillPct int // Weapons + Gunnery (v1: Gunnery applied to all types)
	CritPct        int // Weapons × 1
	Weapons        []Weapon
}

// LoadFit reads a fitting-spec JSON file.
func LoadFit(path string) (*FitSpec, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f FitSpec
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &f, nil
}

// Resolve turns a fit + skills into the combat stat block. Measured rule: no
// capacity skill multipliers — server stat blocks equal catalog + modules.
func Resolve(fit *FitSpec, cat *Catalog) (*StatBlock, error) {
	return resolveFit(fit, cat, false)
}

// ResolveHull resolves a hull's stock loadout (its default_modules). When
// allowCapital is true the tier>=5 guard is skipped so any warship can be a
// defender; NO capital weapon bonus is applied (stated assumption). Used for
// swarm defenders and starter attackers alike.
func ResolveHull(hullID string, cat *Catalog, allowCapital bool) (*StatBlock, error) {
	hull, ok := cat.Ships[hullID]
	if !ok {
		return nil, fmt.Errorf("unknown hull %q", hullID)
	}
	fit := &FitSpec{Name: hull.Name, Hull: hullID, Modules: hull.DefaultModules}
	return resolveFit(fit, cat, allowCapital)
}

func resolveFit(fit *FitSpec, cat *Catalog, allowCapital bool) (*StatBlock, error) {
	hull, ok := cat.Ships[fit.Hull]
	if !ok {
		return nil, fmt.Errorf("unknown hull %q", fit.Hull)
	}
	if hull.Tier >= 5 && !allowCapital {
		return nil, fmt.Errorf("hull %q: capital hulls unsupported as attacker in v1", fit.Hull)
	}
	sk := func(name string) int { return fit.Skills[name] }
	sb := &StatBlock{
		Name:           fit.Name,
		MaxHull:        hull.BaseHull,
		MaxShield:      hull.BaseShield,
		Recharge:       hull.BaseShieldRecharge,
		ShieldsSkill:   sk("shields"),
		WeaponSkillPct: sk("weapons") + sk("gunnery"),
		CritPct:        sk("weapons"),
	}
	armor := hull.BaseArmor
	flat := 0
	for _, id := range fit.Modules {
		it, ok := cat.Items[id]
		if !ok {
			return nil, fmt.Errorf("unknown module %q", id)
		}
		sb.MaxShield += it.ShieldBonus
		sb.Recharge += it.ShieldRechargeBonus
		armor += it.ArmorBonus
		flat += it.DamageReduction
		if it.Slot == "weapon" && it.Damage > 0 {
			sb.Weapons = append(sb.Weapons, Weapon{
				Name: it.ID, Damage: it.Damage, Type: it.DamageType,
				Cooldown: it.Cooldown, Magazine: it.MagazineSize, Reach: it.Reach,
			})
		}
	}
	sb.FlatPct = min(flat, 75)
	sb.ArmorTotal = float64(armor) * (1 + float64(sk("armor"))*0.01)
	for i, w := range sb.Weapons {
		if _, ok := shieldEff[w.Type]; !ok {
			return nil, fmt.Errorf("unknown damage type %q on %s", w.Type, w.Name)
		}
		if i > 0 && w.Type != sb.Weapons[0].Type {
			return nil, fmt.Errorf("mixed-damage-type fits unsupported in v1 (got %s and %s)", sb.Weapons[0].Type, w.Type)
		}
	}
	return sb, nil
}
