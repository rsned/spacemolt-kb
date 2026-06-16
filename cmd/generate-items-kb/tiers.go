package main

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"

	"github.com/rsned/spacemolt-kb/internal/kbnav"
	"github.com/rsned/spacemolt-kb/pkg/tierchart"
)

// itemToTierStats projects an Item onto the storage-agnostic TierStats the
// tierchart package consumes. Zero-valued stats are harmless: tierchart
// suppresses any column that is zero across an entire family.
func itemToTierStats(it *Item) tierchart.TierStats {
	return tierchart.TierStats{
		ItemID:     it.ID,
		ItemName:   it.Name,
		Category:   it.Category,
		BaseValue:  it.BaseValue,
		DamageType: it.DamageType,
		SkillReq:   strings.Join(requiredSkillStrings(it.RequiredSkills), ", "),
		Stats: map[string]int{
			"mining_power":    it.MiningPower,
			"survey_power":    it.SurveyPower,
			"survey_range":    it.SurveyRange,
			"damage":          it.Damage,
			"range":           it.WeaponRange,
			"reach":           it.Reach,
			"cpu":             it.CPUUsage,
			"power":           it.PowerUsage,
			"cooldown":        it.Cooldown,
			"accuracy":        it.AccuracyBonus,
			"tracking":        it.TrackingBonus,
			"armor":           it.ArmorBonus,
			"hull":            it.HullBonus,
			"shield":          it.ShieldBonus,
			"shield_recharge": it.ShieldRechargeBonus,
			"speed":           it.SpeedBonus,
			"cargo":           it.CargoBonus,
			"scanner":         it.ScannerPower,
			"cpu_bonus":       it.CPUBonus,
			"drone_bandwidth": it.DroneBandwidth,
			"drone_capacity":  it.DroneCapacity,
			"max_fuel":        it.MaxFuelBonus,
		},
	}
}

// writeTierComparisonPage renders kb/items/tiers/index.html: one comparison
// table per tiered module family (Mining Laser, Pulse Laser, ...), grouped by
// category. Returns the number of families rendered.
func writeTierComparisonPage(outDir string, items map[string]*Item) (int, error) {
	stats := make([]tierchart.TierStats, 0, len(items))
	for _, it := range items {
		stats = append(stats, itemToTierStats(it))
	}
	families := tierchart.BuildFamilies(stats)

	var body strings.Builder
	var lastCat string
	for _, fam := range families {
		if fam.Category != lastCat {
			fmt.Fprintf(&body, "        <h3 class=\"mt-4\">%s</h3>\n", html.EscapeString(titleCase(fam.Category)))
			lastCat = fam.Category
		}
		body.WriteString(renderFamilyTable(fam))
	}

	page := strings.NewReplacer(
		"{{HEADER}}", kbnav.Header("../../"),
		"{{BODY}}", body.String(),
		"{{SORT}}", sortScript,
		"{{THEME}}", themeScript,
		"{{COUNT}}", fmt.Sprintf("%d", len(families)),
	).Replace(tierPageTemplate)

	tiersDir := filepath.Join(outDir, "tiers")
	if err := os.MkdirAll(tiersDir, 0o755); err != nil {
		return 0, err
	}
	if err := os.WriteFile(filepath.Join(tiersDir, "index.html"), []byte(page), 0o644); err != nil {
		return 0, err
	}
	return len(families), nil
}

// skillReqDisplay renders a required-skills cell, using an em-dash for tiers
// in a family where some other tier has a requirement but this one does not.
func skillReqDisplay(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// renderFamilyTable builds the card + comparison table for one tier family.
func renderFamilyTable(fam tierchart.TierFamily) string {
	cols := fam.Columns()
	showSkill := fam.HasSkillReq()

	var sb strings.Builder
	sb.WriteString(`        <div class="card mt-3" style="padding:0">` + "\n")
	fmt.Fprintf(&sb, "          <div class=\"section-label\">%s Comparison</div>\n", html.EscapeString(fam.DisplayName))
	sb.WriteString(`          <table class="sortable">` + "\n")

	// Header row.
	sb.WriteString(`          <thead><tr><th class="sortable">Model</th>`)
	for _, col := range cols {
		fmt.Fprintf(&sb, `<th class="sortable" style="text-align:right">%s</th>`, html.EscapeString(tierchart.ColumnLabel(col)))
	}
	if showSkill {
		sb.WriteString(`<th class="sortable">Skill Req</th>`)
	}
	sb.WriteString(`<th class="sortable" style="text-align:right">Value</th></tr></thead>` + "\n")

	// Body rows.
	sb.WriteString("          <tbody>\n")
	for _, tier := range fam.Tiers {
		sb.WriteString("          <tr>")
		fmt.Fprintf(&sb, `<td><a href="../%s/%s.html">%s</a></td>`,
			html.EscapeString(tier.Category), html.EscapeString(tier.ItemID), html.EscapeString(tier.ItemName))
		for _, col := range cols {
			fmt.Fprintf(&sb, `<td style="text-align:right" data-sort="%d">%s</td>`,
				tier.Value(col), html.EscapeString(tier.Display(col)))
		}
		if showSkill {
			fmt.Fprintf(&sb, `<td>%s</td>`, html.EscapeString(skillReqDisplay(tier.SkillReq)))
		}
		fmt.Fprintf(&sb, `<td class="value" style="text-align:right" data-sort="%d">%s</td>`,
			tier.BaseValue, fmtValue(tier.BaseValue))
		sb.WriteString("</tr>\n")
	}
	sb.WriteString("          </tbody>\n")
	sb.WriteString("          </table>\n")
	sb.WriteString("        </div>\n")
	return sb.String()
}

var tierPageTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Tier Comparison - Items - Spacemolt KB</title>
    <link rel="stylesheet" href="../../smui.css">
    <link rel="stylesheet" href="../../items/items.css">
</head>
<body>
{{HEADER}}
    <main class="container page-content">
        <div class="breadcrumb"><a href="../">Items</a> / Tier Comparison</div>
        <h2>Module Tier Comparison</h2>
        <p class="text-muted mt-1">Side-by-side stats for {{COUNT}} tiered module families. Click a column header to sort.</p>
{{BODY}}
    </main>
{{SORT}}
{{THEME}}
</body>
</html>
`
