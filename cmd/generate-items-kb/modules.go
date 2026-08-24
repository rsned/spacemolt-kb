package main

import (
	"cmp"
	"fmt"
	htmltpl "html/template"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Module comparison pages.
//
// The per-category item index (htmlCatTemplate) lists only name/rarity/size/
// value/description, and the tier page compares within one family at a time
// (Autocannon I -> III). Neither answers the actual fitting question, which is
// cross-family and budget-constrained: given this much free CPU and power,
// which weapon buys the most damage, and which defense covers the damage type
// that is actually killing me. These two pages are that comparison.

// damageTypes is the fixed column order for per-type resistance columns, and
// the filter-chip order on the weapons page. Kept explicit rather than derived
// so the two pages line up column-for-column when read side by side.
var damageTypes = []string{"kinetic", "thermal", "energy", "em", "explosive", "void"}

// ammoVariant is one loadable round for a weapon's ammo type.
type ammoVariant struct {
	Name      string
	ID        string
	Rarity    string
	DamageMod float64
}

// weaponRow is a single row of the all-weapons comparison table.
type weaponRow struct {
	*Item
	DPT        float64 // damage per tick: damage / cooldown
	DPTPerCPU  float64
	DPTPerPwr  float64
	Volley     int // damage * magazine_size, 0 when the weapon takes no ammo
	EffMinDPT  float64
	EffMaxDPT  float64
	AmmoCount  int
	SkillReq   string
	Specials   []string
}

// defenseRow is a single row of the all-defense comparison table.
type defenseRow struct {
	*Item
	Resists   []string // per damageTypes order; "" when the module has none
	Penalties []string
	SkillReq  string
	Specials  []string
}

// buildAmmoIndex maps a weapon ammo_type to the rounds that fit it, so the
// weapons table can show effective damage as a range rather than pretending
// the base number is what you actually field.
func buildAmmoIndex(items []*Item) map[string][]ammoVariant {
	idx := map[string][]ammoVariant{}
	for _, it := range items {
		if it.Category != "ammo" || it.Effect == nil || it.Effect.Type != "ammo" {
			continue
		}
		sub := it.Effect.Subtype
		if sub == "" {
			continue
		}
		v := ammoVariant{Name: it.Name, ID: it.ID, Rarity: it.Rarity}
		if raw, ok := it.Effect.Ammo["damage_mod"]; ok {
			if f, ok := toFloat(raw); ok {
				v.DamageMod = f
			}
		}
		idx[sub] = append(idx[sub], v)
	}
	for k := range idx {
		slices.SortFunc(idx[k], func(a, b ammoVariant) int { return cmp.Compare(a.DamageMod, b.DamageMod) })
	}
	return idx
}

// toFloat coerces a JSON number out of an any without panicking on the
// int/float ambiguity encoding/json leaves behind.
func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	}
	return 0, false
}

// specialLabels maps the catalog's special-token prefixes to readable text.
// Tokens are comma-separated and usually carry a trailing magnitude
// (armor_bypass_50), which decodeSpecials splits off and re-appends.
var specialLabels = map[string]string{
	"adaptive_resistance":   "Adaptive resistance",
	"aoe_radius":            "AoE radius",
	"anti_drone_bonus":      "Anti-drone",
	"anti_missile_bonus":    "Anti-missile",
	"armor_bypass":          "Armor bypass",
	"armor_melt":            "Armor melt",
	"capacitor_drain":       "Capacitor drain",
	"capacitor_transfer":    "Capacitor transfer",
	"chain_lightning":       "Chain lightning",
	"cpu_damage":            "CPU damage",
	"energy_damage_bonus":   "Energy damage",
	"hull_damage_bonus":     "Hull damage",
	"ignores_resistance":    "Ignores resistance",
	"lifesteal":             "Lifesteal",
	"mine_capacity":         "Mine capacity",
	"mine_detection":        "Mine detection",
	"mine_duration":         "Mine duration",
	"mine_tracking_speed":   "Mine tracking",
	"module_disable":        "Module disable",
	"phase_dodge":           "Phase dodge",
	"phase_strike":          "Phase strike",
	"random_damage_variance": "Damage variance",
	"reflect_energy":        "Reflect energy",
	"shield_bypass":         "Shield bypass",
	"shield_damage_bonus":   "Shield damage",
	"shock_attackers":       "Shock attackers",
	"system_disable":        "System disable",
}

// specialFlags are the valueless special tokens.
var specialFlags = map[string]string{
	"ammo_from_cargo":          "Feeds from cargo",
	"damage_boost_on_hit":      "Damage boost on hit",
	"emergency_warp_at_20_hull": "Emergency warp at 20% hull",
	"ignores_all_defense":      "Ignores all defense",
	"low_cpu_requirement":      "Low CPU requirement",
	"rage_damage_scaling":      "Rage damage scaling",
	"repair_from_salvage":      "Repairs from salvage",
	"shield_phase":             "Shield phase",
	"target_specific":          "Target specific",
	"common_only":              "Common ammo only",
}

// decodeSpecials turns "armor_bypass_80,hull_damage_bonus_50" into
// ["Armor bypass 80", "Hull damage 50"]. Unknown tokens are title-cased rather
// than dropped, so a server-side addition shows up as readable text instead of
// vanishing from the page.
func decodeSpecials(special string) []string {
	if strings.TrimSpace(special) == "" {
		return nil
	}
	var out []string
	for _, tok := range strings.Split(special, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		if lbl, ok := specialFlags[tok]; ok {
			out = append(out, lbl)
			continue
		}
		// Split a trailing numeric magnitude off the prefix.
		prefix, mag := tok, ""
		if i := strings.LastIndex(tok, "_"); i > 0 {
			if _, err := strconv.Atoi(tok[i+1:]); err == nil {
				prefix, mag = tok[:i], tok[i+1:]
			}
		}
		lbl, ok := specialLabels[prefix]
		if !ok {
			lbl = titleCase(strings.ReplaceAll(prefix, "_", " "))
		}
		if mag != "" {
			lbl += " " + mag
		}
		out = append(out, lbl)
	}
	return out
}

// skillReqString renders the required-skills map deterministically.
func skillReqString(m map[string]int) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s %d", titleCase(strings.ReplaceAll(k, "_", " ")), m[k]))
	}
	return strings.Join(parts, ", ")
}

// div guards the divide-by-zero cases (a module with no cooldown, or a passive
// with no CPU cost) so the table shows a dash instead of +Inf.
func div(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

// buildWeaponRows computes the derived fitting columns for every weapon.
func buildWeaponRows(items []*Item, ammo map[string][]ammoVariant) []weaponRow {
	var rows []weaponRow
	for _, it := range items {
		if it.Slot != "weapon" {
			continue
		}
		r := weaponRow{Item: it}
		r.DPT = div(float64(it.Damage), float64(it.Cooldown))
		r.DPTPerCPU = div(r.DPT, float64(it.CPUUsage))
		r.DPTPerPwr = div(r.DPT, float64(it.PowerUsage))
		r.Volley = it.Damage * it.MagazineSize
		r.SkillReq = skillReqString(it.RequiredSkills)
		r.Specials = decodeSpecials(it.Special)
		// Effective damage/tick across every round that fits. A weapon with no
		// ammo type is its own floor and ceiling.
		variants := ammo[it.AmmoType]
		r.AmmoCount = len(variants)
		if len(variants) == 0 {
			r.EffMinDPT, r.EffMaxDPT = r.DPT, r.DPT
		} else {
			r.EffMinDPT = r.DPT * (1 + variants[0].DamageMod)
			r.EffMaxDPT = r.DPT * (1 + variants[len(variants)-1].DamageMod)
		}
		rows = append(rows, r)
	}
	slices.SortFunc(rows, func(a, b weaponRow) int { return cmp.Compare(a.Name, b.Name) })
	return rows
}

// buildDefenseRows assembles the defense table, including the per-damage-type
// resistance columns that let it be read against the weapons page.
func buildDefenseRows(items []*Item) []defenseRow {
	var rows []defenseRow
	for _, it := range items {
		if it.Slot != "defense" {
			continue
		}
		r := defenseRow{Item: it}
		r.Resists = make([]string, len(damageTypes))
		for i, dt := range damageTypes {
			if v, ok := it.ResistanceBonus[dt]; ok && v != 0 {
				r.Resists[i] = fmtPct(v)
			}
		}
		if it.SpeedPenalty != 0 {
			r.Penalties = append(r.Penalties, fmt.Sprintf("Speed %d", -abs(it.SpeedPenalty)))
		}
		if it.HullPenalty != 0 {
			r.Penalties = append(r.Penalties, fmt.Sprintf("Hull %d", -abs(it.HullPenalty)))
		}
		if it.TowSpeedPenalty != 0 {
			r.Penalties = append(r.Penalties, fmt.Sprintf("Tow speed %d", -abs(it.TowSpeedPenalty)))
		}
		r.SkillReq = skillReqString(it.RequiredSkills)
		r.Specials = decodeSpecials(it.Special)
		rows = append(rows, r)
	}
	slices.SortFunc(rows, func(a, b defenseRow) int { return cmp.Compare(a.Name, b.Name) })
	return rows
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// fmtPct renders a resistance value, which the catalog supplies either as a
// fraction (0.25) or as whole percent (25).
func fmtPct(v float64) string {
	if v <= 1 {
		v *= 100
	}
	return strconv.FormatFloat(v, 'f', -1, 64) + "%"
}

// writeModuleComparisons emits weapon/all.html and defense/all.html.
func writeModuleComparisons(outDir string, items []*Item) error {
	ammo := buildAmmoIndex(items)
	weapons := buildWeaponRows(items, ammo)
	defenses := buildDefenseRows(items)

	funcs := htmltpl.FuncMap{
		"titleCase": titleCase,
		"fmtValue":  fmtValue,
		"num1": func(f float64) string {
			if f == 0 {
				return "—"
			}
			return strconv.FormatFloat(f, 'f', 1, 64)
		},
		"num2": func(f float64) string {
			if f == 0 {
				return "—"
			}
			return strconv.FormatFloat(f, 'f', 2, 64)
		},
		"intOrDash": func(n int) string {
			if n == 0 {
				return "—"
			}
			return strconv.Itoa(n)
		},
		// pctOrDash renders damage_reduction, which the catalog supplies as a
		// whole percent (10), not a fraction.
		"pctOrDash": func(f float64) string {
			if f == 0 {
				return "—"
			}
			return strconv.FormatFloat(f, 'f', -1, 64) + "%"
		},
		"strOrDash": func(s string) string {
			if s == "" {
				return "—"
			}
			return s
		},
		"effRange": func(r weaponRow) string {
			if r.EffMinDPT == r.EffMaxDPT {
				return strconv.FormatFloat(r.DPT, 'f', 1, 64)
			}
			return strconv.FormatFloat(r.EffMinDPT, 'f', 1, 64) + "–" + strconv.FormatFloat(r.EffMaxDPT, 'f', 1, 64)
		},
		"dtClass":     func(dt string) string { return "dt-" + dt },
		// dtLabel title-cases a damage type, keeping EM as an initialism
		// rather than the "Em" titleCase would produce.
		"dtLabel": func(dt string) string {
			if dt == "em" {
				return "EM"
			}
			return titleCase(dt)
		},
		"damageTypes": func() []string { return damageTypes },
		"lower":       strings.ToLower,
		// ammoLabel title-cases an ammo type, keeping EM an initialism.
		"ammoLabel": func(a string) string {
			if a == "" {
				return ""
			}
			return strings.ReplaceAll(titleCase(a), "Em ", "EM ")
		},
		"join":        func(v []string, sep string) string { return strings.Join(v, sep) },
		// resistedTypes lists the damage types a defense module actually
		// resists, space-joined, so the filter chips can match any of them.
		"resistedTypes": func(r defenseRow) string {
			var out []string
			for i, v := range r.Resists {
				if v != "" {
					out = append(out, damageTypes[i])
				}
			}
			return strings.Join(out, " ")
		},
		// pctSort turns "25%" into a sortable number, and an absent
		// resistance into 0 so blanks sort below real values.
		"pctSort": func(s string) string {
			if s == "" {
				return "0"
			}
			return strings.TrimSuffix(s, "%")
		},
	}

	// Ammo types actually present on weapons, for the filter select.
	seen := map[string]bool{}
	var ammoTypes []string
	for _, r := range weapons {
		if r.AmmoType != "" && !seen[r.AmmoType] {
			seen[r.AmmoType] = true
			ammoTypes = append(ammoTypes, r.AmmoType)
		}
	}
	sort.Strings(ammoTypes)

	wTmpl := htmltpl.Must(htmltpl.New("weapons-all").Funcs(funcs).Parse(weaponAllTemplate))
	if err := writeTemplate(filepath.Join(outDir, "weapon", "all.html"), wTmpl, struct {
		Rows      []weaponRow
		Total     int
		AmmoTypes []string
	}{weapons, len(weapons), ammoTypes}); err != nil {
		return err
	}

	dTmpl := htmltpl.Must(htmltpl.New("defense-all").Funcs(funcs).Parse(defenseAllTemplate))
	return writeTemplate(filepath.Join(outDir, "defense", "all.html"), dTmpl, struct {
		Rows        []defenseRow
		Total       int
		DamageTypes []string
	}{defenses, len(defenses), damageTypes})
}
