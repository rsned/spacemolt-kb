package main

import (
	"cmp"
	"encoding/json"
	"fmt"
	htmltpl "html/template"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Ship holds a ship class from the catalog JSON.
type Ship struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Category          string   `json:"category"`
	Class             string   `json:"class"`
	Faction           string   `json:"faction"`
	Tier              int      `json:"tier"`
	BaseHull          int      `json:"base_hull"`
	BaseArmor         int      `json:"base_armor"`
	BaseShield        int      `json:"base_shield"`
	BaseShieldRecharge int     `json:"base_shield_recharge"`
	BaseSpeed         int      `json:"base_speed"`
	BaseFuel          int      `json:"base_fuel"`
	CargoCapacity     int      `json:"cargo_capacity"`
	WeaponSlots       int      `json:"weapon_slots"`
	DefenseSlots      int      `json:"defense_slots"`
	UtilitySlots      int      `json:"utility_slots"`
	PassiveRecipes    []string `json:"passive_recipes"`
}

type shipCategory struct {
	Name    string
	Slug    string
	Classes []shipClass
}

type shipClass struct {
	Name     string
	Slug     string
	Factions []shipFaction
}

type shipFaction struct {
	Name  string
	Badge string
	Ships []*Ship
}

func loadShipCatalog(catalogPath string) ([]*Ship, error) {
	data, err := os.ReadFile(catalogPath)
	if err != nil {
		return nil, err
	}
	var catalog struct {
		Items []*Ship `json:"items"`
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil, err
	}
	return catalog.Items, nil
}

func writeShipPages(outDir string, ships []*Ship, recipeNames map[string]string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	// Group: category -> class -> faction -> ships.
	catMap := make(map[string]map[string]map[string][]*Ship)
	for _, s := range ships {
		if catMap[s.Category] == nil {
			catMap[s.Category] = make(map[string]map[string][]*Ship)
		}
		if catMap[s.Category][s.Class] == nil {
			catMap[s.Category][s.Class] = make(map[string][]*Ship)
		}
		catMap[s.Category][s.Class][s.Faction] = append(catMap[s.Category][s.Class][s.Faction], s)
	}

	// Build structured data.
	var categories []shipCategory
	for catName, clsMap := range catMap {
		cat := shipCategory{
			Name: catName,
			Slug: slugify(catName),
		}
		for clsName, factionMap := range clsMap {
			cls := shipClass{
				Name: clsName,
				Slug: slugify(catName) + "--" + slugify(clsName),
			}
			for factionName, factionShips := range factionMap {
				slices.SortFunc(factionShips, func(a, b *Ship) int {
					if a.Tier != b.Tier {
						return cmp.Compare(a.Tier, b.Tier)
					}
					return cmp.Compare(a.Name, b.Name)
				})
				cls.Factions = append(cls.Factions, shipFaction{
					Name:  factionDisplayName(factionName),
					Badge: factionBadgeClass(factionName),
					Ships: factionShips,
				})
			}
			slices.SortFunc(cls.Factions, func(a, b shipFaction) int { return cmp.Compare(a.Name, b.Name) })
			cat.Classes = append(cat.Classes, cls)
		}
		slices.SortFunc(cat.Classes, func(a, b shipClass) int { return cmp.Compare(a.Name, b.Name) })
		categories = append(categories, cat)
	}
	slices.SortFunc(categories, func(a, b shipCategory) int { return cmp.Compare(a.Name, b.Name) })

	// Count totals.
	totalShips := len(ships)
	totalCats := len(categories)
	totalFactions := 5

	funcs := htmltpl.FuncMap{
		"fmtNum":  func(n int) string { return fmt.Sprintf("%d", n) },
		"slugify": slugify,
		"recipeName": func(id string) string {
			if n, ok := recipeNames[id]; ok {
				return n
			}
			return id
		},
		"hasPassive": func(s *Ship) bool { return len(s.PassiveRecipes) > 0 },
		"classShipCount": func(c shipClass) int {
			n := 0
			for _, f := range c.Factions {
				n += len(f.Ships)
			}
			return n
		},
	}

	type pageData struct {
		Categories    []shipCategory
		TotalShips    int
		TotalCats     int
		TotalFactions int
	}

	tmpl := htmltpl.Must(htmltpl.New("ships").Funcs(funcs).Parse(shipPageTemplate))
	return writeTemplate(filepath.Join(outDir, "index.html"), tmpl, pageData{
		Categories:    categories,
		TotalShips:    totalShips,
		TotalCats:     totalCats,
		TotalFactions: totalFactions,
	})
}

func slugify(s string) string {
	return strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(s, " ", "-"), "_", "-"))
}

func factionDisplayName(f string) string {
	switch f {
	case "crimson":
		return "Crimson"
	case "nebula":
		return "Nebula"
	case "outerrim":
		return "Outer Rim"
	case "solarian":
		return "Solarian"
	case "voidborn":
		return "Voidborn"
	default:
		return f
	}
}

func factionBadgeClass(f string) string {
	switch f {
	case "crimson":
		return "badge-red"
	case "nebula":
		return "badge-purple"
	case "outerrim":
		return "badge-orange"
	case "solarian":
		return "badge-frost"
	case "voidborn":
		return "badge-green"
	default:
		return "badge-frost"
	}
}

var shipPageTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Ships - Spacemolt KB</title>
    <link rel="stylesheet" href="../smui.css">
    <link rel="stylesheet" href="ships.css">
</head>
<body>
` + siteHeader + `
    <main class="container page-content">
      <h2>Ships</h2>
      <p class="text-muted mt-1">{{.TotalShips}} ship classes across {{.TotalCats}} categories and {{.TotalFactions}} factions.</p>

      <nav class="card mt-3" id="toc">
        <div class="card-header"><span class="label">Categories</span></div>
        <ul class="toc-list">
{{- range .Categories}}
            <li><a href="#{{slugify .Name}}">{{.Name}}</a> <span class="text-muted">({{len .Classes}} classes)</span></li>
{{- end}}
        </ul>
      </nav>
{{range .Categories}}
      <section class="ship-category mt-3" id="{{slugify .Name}}">
        <h2>{{.Name}} <span class="text-muted" style="font-size:var(--text-ui)">({{len .Classes}} classes)</span></h2>
        <ul class="class-toc">
{{- range .Classes}}
            <li><a href="#{{.Slug}}">{{.Name}}</a> <span class="text-muted">({{classShipCount .}})</span></li>
{{- end}}
        </ul>
{{- range .Classes}}
        <section class="ship-class mt-3" id="{{.Slug}}">
          <h3>{{.Name}}</h3>
{{- range .Factions}}
          <div class="faction-group mt-2">
            <h4><span class="badge {{.Badge}}">{{.Name}}</span></h4>
            <table>
              <thead>
                <tr>
                  <th>Name</th><th>Tier</th><th>Hull</th><th>Armor</th><th>Shield</th><th>Shd Regen/t</th><th>Speed</th><th>Fuel</th><th>Cargo</th><th>Wpn</th><th>Def</th><th>Util</th>
                </tr>
              </thead>
              <tbody>
{{- range .Ships}}
            <tr>
              <td>{{.Name}}{{if hasPassive .}} <span class="badge badge-green" title="Passive Recipes: {{range $i, $r := .PassiveRecipes}}{{if $i}}, {{end}}{{recipeName $r}}{{end}}">&#x2692; Passive</span>{{end}}</td>
              <td class="num">{{.Tier}}</td>
              <td class="num">{{.BaseHull}}</td>
              <td class="num">{{.BaseArmor}}</td>
              <td class="num">{{.BaseShield}}</td>
              <td class="num">{{.BaseShieldRecharge}}/t</td>
              <td class="num">{{.BaseSpeed}} AU/t</td>
              <td class="num">{{.BaseFuel}}</td>
              <td class="num">{{.CargoCapacity}}</td>
              <td class="num">{{.WeaponSlots}}</td>
              <td class="num">{{.DefenseSlots}}</td>
              <td class="num">{{.UtilitySlots}}</td>
            </tr>
{{- end}}
              </tbody>
            </table>
          </div>
{{- end}}
        </section>
{{- end}}
      </section>
{{end}}
    </main>
` + themeScript + `
</body>
</html>
`
