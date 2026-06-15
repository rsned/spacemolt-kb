// Package tierchart groups tiered module families (e.g. pulse_laser I-III,
// mining_laser I-V) and decides which stat columns are relevant for each
// family's comparison table. It is deliberately storage-agnostic: callers
// populate TierStats from whatever item source they have (DB row, catalog
// JSON, in-memory struct) and tierchart handles grouping, ordering, and
// column relevance.
package tierchart

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
)

// columnOrder is the canonical left-to-right order of stat columns in a
// comparison table. Only columns with a non-zero value in at least one tier
// are actually rendered (see TierFamily.Columns). The "Model" column is always
// first and the "Value" column always last; neither appears here.
var columnOrder = []string{
	"mining_power",
	"survey_power",
	"survey_range",
	"damage",
	"range",
	"reach",
	"cpu",
	"power",
	"cooldown",
	"accuracy",
	"tracking",
	"armor",
	"hull",
	"shield",
	"shield_recharge",
	"speed",
	"cargo",
	"scanner",
	"cpu_bonus",
	"drone_bandwidth",
	"drone_capacity",
	"max_fuel",
}

// columnLabels maps a column key to its human-readable header.
var columnLabels = map[string]string{
	"mining_power":    "Mining Power",
	"survey_power":    "Survey Power",
	"survey_range":    "Survey Range",
	"damage":          "Damage",
	"range":           "Range",
	"reach":           "Reach",
	"cpu":             "CPU",
	"power":           "Power",
	"cooldown":        "Cooldown",
	"accuracy":        "Accuracy",
	"tracking":        "Tracking",
	"armor":           "Armor",
	"hull":            "Hull",
	"shield":          "Shield",
	"shield_recharge": "Shield Regen",
	"speed":           "Speed",
	"cargo":           "Cargo",
	"scanner":         "Scanner",
	"cpu_bonus":       "CPU Bonus",
	"drone_bandwidth": "Drone Bandwidth",
	"drone_capacity":  "Drone Capacity",
	"max_fuel":        "Max Fuel",
}

// ColumnLabel returns the human-readable header for a column key, or the key
// itself if unknown.
func ColumnLabel(col string) string {
	if l, ok := columnLabels[col]; ok {
		return l
	}
	return col
}

// TierStats holds the comparison stats for a single tier of a module. Stats is
// keyed by the column keys in columnOrder; absent or zero values are treated as
// "not applicable" and suppress the column when zero across the whole family.
type TierStats struct {
	Tier       string // I, II, III, IV, V
	ItemID     string // full item ID (e.g. pulse_laser_i)
	ItemName   string // display name
	Category   string // item category (weapon, mining, ...)
	BaseValue  int    // cost in credits
	DamageType string // weapon damage type, appended to the damage cell when set
	Stats      map[string]int
}

// Value returns the raw integer for a column (0 if absent).
func (t TierStats) Value(col string) int { return t.Stats[col] }

// Display returns the formatted cell text for a column. The damage column
// appends the damage type (e.g. "28 energy") when one is present.
func (t TierStats) Display(col string) string {
	v := t.Stats[col]
	if col == "damage" && t.DamageType != "" {
		return strconv.Itoa(v) + " " + t.DamageType
	}
	return strconv.Itoa(v)
}

// TierFamily is a complete tier progression for one module base name.
type TierFamily struct {
	BaseName    string // e.g. "pulse_laser"
	DisplayName string // e.g. "Pulse Laser"
	Category    string // category of the first tier
	Tiers       []TierStats
}

// Columns returns the relevant stat columns for this family in canonical order.
// A column is relevant when at least one tier has a non-zero value for it.
func (f TierFamily) Columns() []string {
	var cols []string
	for _, col := range columnOrder {
		for _, t := range f.Tiers {
			if t.Stats[col] != 0 {
				cols = append(cols, col)
				break
			}
		}
	}
	return cols
}

// BuildFamilies groups the supplied tier stats by base name, sorts each
// family's tiers by roman-numeral order, and drops singletons (a "family" needs
// at least two tiers). The result is sorted by display name. Stats whose ID has
// no recognized roman-numeral suffix are ignored.
func BuildFamilies(stats []TierStats) []TierFamily {
	groups := map[string][]TierStats{}
	for _, s := range stats {
		base := extractBaseName(s.ItemID)
		if base == "" {
			continue
		}
		s.Tier = extractTier(s.ItemID)
		groups[base] = append(groups[base], s)
	}

	var families []TierFamily
	for base, tiers := range groups {
		if len(tiers) < 2 {
			continue
		}
		slices.SortFunc(tiers, func(a, b TierStats) int {
			return cmp.Compare(tierOrder(a.Tier), tierOrder(b.Tier))
		})
		// Prefer the server's own capitalization (e.g. "EMP Pulse") by stripping
		// the tier label off the first member's name; fall back to the base ID.
		display := strings.TrimSpace(strings.TrimSuffix(tiers[0].ItemName, " "+tiers[0].Tier))
		if display == "" {
			display = displayName(base)
		}
		families = append(families, TierFamily{
			BaseName:    base,
			DisplayName: display,
			Category:    tiers[0].Category,
			Tiers:       tiers,
		})
	}

	slices.SortFunc(families, func(a, b TierFamily) int {
		if c := cmp.Compare(a.Category, b.Category); c != 0 {
			return c
		}
		return cmp.Compare(a.DisplayName, b.DisplayName)
	})
	return families
}

// romanSuffixes lists the recognized tier suffixes longest-first so that, e.g.,
// "_iii" is matched before "_ii" and "_i".
var romanSuffixes = []struct {
	suffix string
	tier   string
	rank   int
}{
	{"_viii", "VIII", 8},
	{"_vii", "VII", 7},
	{"_vi", "VI", 6},
	{"_iv", "IV", 4},
	{"_v", "V", 5},
	{"_iii", "III", 3},
	{"_ii", "II", 2},
	{"_i", "I", 1},
}

// extractBaseName strips a roman-numeral tier suffix from an item ID.
// e.g. "pulse_laser_i" -> "pulse_laser". Returns "" if there is no suffix.
func extractBaseName(id string) string {
	for _, s := range romanSuffixes {
		if base, ok := strings.CutSuffix(id, s.suffix); ok {
			return base
		}
	}
	return ""
}

// extractTier returns the roman-numeral tier label for an item ID
// (e.g. "mining_laser_iii" -> "III"), or "" if there is no suffix.
func extractTier(id string) string {
	for _, s := range romanSuffixes {
		if strings.HasSuffix(id, s.suffix) {
			return s.tier
		}
	}
	return ""
}

// tierOrder maps a roman-numeral tier to its numeric rank for sorting.
func tierOrder(tier string) int {
	for _, s := range romanSuffixes {
		if s.tier == tier {
			return s.rank
		}
	}
	return 0
}

// displayName converts a base name into a title-cased display string.
// e.g. "mining_laser" -> "Mining Laser".
func displayName(base string) string {
	words := strings.Split(base, "_")
	for i, w := range words {
		if w == "" {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}
