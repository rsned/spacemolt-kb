package main

import (
	"cmp"
	"encoding/json"
	htmltpl "html/template"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	humanize "github.com/dustin/go-humanize"
)

// Skill holds a skill loaded from the catalog JSON.
type Skill struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Description      string            `json:"description"`
	Category         string            `json:"category"`
	MaxLevel         int               `json:"max_level"`
	TrainingSource   string            `json:"training_source"`
	XPPerLevel       []int             `json:"xp_per_level"`
	BonusPerLevel    map[string]int    `json:"bonus_per_level"`
	EmpireRestrict   string            `json:"empire_restriction"`
}

// SkillCategoryInfo groups skills for page generation.
type SkillCategoryInfo struct {
	Name   string
	Skills []*Skill
}

func loadSkills(catalogPath string) ([]*Skill, error) {
	data, err := os.ReadFile(catalogPath)
	if err != nil {
		return nil, err
	}
	var catalog struct {
		Items []*Skill `json:"items"`
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil, err
	}
	return catalog.Items, nil
}

func writeSkillPages(outDir string, skills []*Skill) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	// Clean old HTML files (preserve CSS).
	entries, _ := os.ReadDir(outDir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".html") {
			_ = os.Remove(filepath.Join(outDir, e.Name()))
		}
	}

	// Group by category.
	catSkills := make(map[string][]*Skill)
	for _, s := range skills {
		catSkills[s.Category] = append(catSkills[s.Category], s)
	}
	for _, list := range catSkills {
		slices.SortFunc(list, func(a, b *Skill) int { return cmp.Compare(a.Name, b.Name) })
	}

	categories := make([]SkillCategoryInfo, 0, len(catSkills))
	for cat, list := range catSkills {
		categories = append(categories, SkillCategoryInfo{Name: cat, Skills: list})
	}
	slices.SortFunc(categories, func(a, b SkillCategoryInfo) int { return cmp.Compare(a.Name, b.Name) })

	funcs := htmltpl.FuncMap{
		"fmtNum":       func(n int) string { return humanize.Comma(int64(n)) },
		"lower":        strings.ToLower,
		"empireClass":  empireClass,
		"empireName":   empireName,
		"catSlug":      func(s string) string { return "cat_" + strings.ToLower(strings.ReplaceAll(s, " ", "_")) },
		"totalSkills": func(cats []SkillCategoryInfo) int {
			n := 0
			for _, c := range cats {
				n += len(c.Skills)
			}
			return n
		},
	}

	// --- Index page (all categories combined) ---
	indexTmpl := htmltpl.Must(htmltpl.New("idx").Funcs(funcs).Parse(skillIndexTemplate))
	if err := writeTemplate(filepath.Join(outDir, "index.html"), indexTmpl, categories); err != nil {
		return err
	}

	// --- Individual skill pages ---
	detailTmpl := htmltpl.Must(htmltpl.New("detail").Funcs(funcs).Parse(skillDetailTemplate))
	for _, s := range skills {
		type detailData struct {
			*Skill
			XPRows     []xpRow
			BonusNames []string
			BonusRows  []bonusRow
		}

		// XP table.
		xpRows := make([]xpRow, 0, len(s.XPPerLevel))
		cumXP := 0
		for i, xp := range s.XPPerLevel {
			cumXP += xp
			xpRows = append(xpRows, xpRow{Level: i + 1, XP: xp, Cumulative: cumXP})
		}

		// Bonus table.
		bonusNames := make([]string, 0, len(s.BonusPerLevel))
		for k := range s.BonusPerLevel {
			bonusNames = append(bonusNames, k)
		}
		sort.Strings(bonusNames)

		bonusRows := make([]bonusRow, 0, s.MaxLevel)
		for lvl := 1; lvl <= s.MaxLevel; lvl++ {
			row := bonusRow{Level: lvl, Values: make([]int, len(bonusNames))}
			for i, name := range bonusNames {
				row.Values[i] = s.BonusPerLevel[name] * lvl
			}
			bonusRows = append(bonusRows, row)
		}

		data := detailData{
			Skill:      s,
			XPRows:     xpRows,
			BonusNames: bonusNames,
			BonusRows:  bonusRows,
		}

		if err := writeTemplate(filepath.Join(outDir, s.ID+".html"), detailTmpl, data); err != nil {
			return err
		}
	}

	return nil
}

type xpRow struct {
	Level      int
	XP         int
	Cumulative int
}

type bonusRow struct {
	Level  int
	Values []int
}

func empireClass(empire string) string {
	switch empire {
	case "crimson":
		return "badge-red"
	case "solarian":
		return "badge-yellow"
	case "nebula":
		return "badge-purple"
	case "voidborn":
		return "badge-frost"
	case "outer_rim":
		return "badge-orange"
	default:
		return "badge-frost"
	}
}

func empireName(empire string) string {
	parts := strings.Split(empire, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}


func writeTemplate(path string, tmpl *htmltpl.Template, data any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := tmpl.Execute(f, data); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// --- Templates ---

var skillIndexTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Skills - Spacemolt KB</title>
    <link rel="stylesheet" href="../smui.css">
    <link rel="stylesheet" href="../skills/skills.css">
</head>
<body>
` + siteHeader + `
    <main class="container page-content">
        <h2>Skills</h2>
        <p class="text-muted mt-1">{{totalSkills .}} skills across {{len .}} categories.</p>
        <nav class="toc">
{{- range .}}
            <a href="#{{catSlug .Name}}">{{.Name}}</a> <span class="text-muted">({{len .Skills}})</span>
{{- end}}
        </nav>
{{- range .}}
        <h3 id="{{catSlug .Name}}">{{.Name}}</h3>
        <div class="card" style="padding:0">
        <table class="skill-table">
        <thead><tr><th>Skill</th><th>Max Level</th><th>Description</th></tr></thead>
        <tbody>
{{- range .Skills}}
        <tr>
          <td><a href="{{.ID}}.html">{{.Name}}</a>{{if .EmpireRestrict}} <span class="badge {{empireClass .EmpireRestrict}}" title="{{empireName .EmpireRestrict}} Only">{{empireName .EmpireRestrict}}</span>{{end}}</td>
          <td>{{.MaxLevel}}</td>
          <td>{{.Description}}</td>
        </tr>
{{- end}}
        </tbody>
        </table>
        </div>
{{- end}}
    </main>
` + themeScript + `
</body>
</html>
`

var skillDetailTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Name}} - Spacemolt KB</title>
    <link rel="stylesheet" href="../smui.css">
    <link rel="stylesheet" href="../skills/skills.css">
</head>
<body>
` + siteHeader + `
    <main class="container page-content">
        <div class="breadcrumb"><a href="index.html">Skills</a> / <a href="index.html#{{catSlug .Category}}">{{.Category}}</a> / {{.Name}}</div>
        <h2>{{.Name}}{{if .EmpireRestrict}} <span class="badge {{empireClass .EmpireRestrict}}" title="Empire Restricted: {{empireName .EmpireRestrict}}">{{empireName .EmpireRestrict}} Only</span>{{end}}</h2>
        <p class="text-muted mt-1">{{.Description}}</p>
        <dl class="skill-meta">
            <dt>Category</dt>
            <dd><a href="index.html#{{catSlug .Category}}">{{.Category}}</a></dd>
{{- if .EmpireRestrict}}
            <dt>Empire</dt>
            <dd><span class="badge {{empireClass .EmpireRestrict}}">{{empireName .EmpireRestrict}}</span></dd>
{{- end}}
            <dt>Max Level</dt>
            <dd>{{.MaxLevel}}</dd>
            <dt>Training</dt>
            <dd>{{if .TrainingSource}}{{.TrainingSource}}{{else}}Not yet available.{{end}}</dd>
        </dl>
        <div class="level-tables"><div>
            <h3>XP per Level</h3>
            <table>
                <thead><tr><th>Level</th><th>XP Required</th><th>Cumulative</th></tr></thead>
                <tbody>{{range .XPRows}}<tr><td>{{.Level}}</td><td class="num">{{fmtNum .XP}}</td><td class="num">{{fmtNum .Cumulative}}</td></tr>{{end}}</tbody>
            </table>
        </div>{{if .BonusNames}}<div>
            <h3>Bonuses per Level</h3>
            <table>
                <thead><tr><th>Level</th>{{range .BonusNames}}<th>{{.}}</th>{{end}}</tr></thead>
                <tbody>{{range .BonusRows}}<tr><td>{{.Level}}</td>{{range .Values}}<td class="num">+{{.}}</td>{{end}}</tr>{{end}}</tbody>
            </table>
        </div>{{end}}</div>

    </main>
` + themeScript + `
</body>
</html>
`
