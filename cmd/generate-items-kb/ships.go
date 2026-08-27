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

	"github.com/rsned/spacemolt-kb/internal/kblegacy"
	"github.com/rsned/spacemolt-kb/pkg/bom"
)

// legacySet lists catalog entries the game no longer sells. Loaded once in
// main; an absent sidecar simply yields empty maps, so generation still works
// on a checkout that has not run scripts/build_legacy.py.
var legacySet kblegacy.Set

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
	PowerCapacity     int      `json:"power_capacity"`
	CPUCapacity       int      `json:"cpu_capacity"`
	Scale             int      `json:"scale"`
	BuildTime         int      `json:"build_time"`
	Price             int      `json:"price"`
	ShipyardTier      int      `json:"shipyard_tier"`
	StarterShip       bool     `json:"starter_ship"`
	Description       string   `json:"description"`
	Lore              string   `json:"lore"`
	FlavorTags        []string `json:"flavor_tags"`
	DefaultModules    []string `json:"default_modules"`
	BuildMaterials    []struct {
		ItemID       string `json:"item_id"`
		Quantity     int    `json:"quantity"`
		ItemCategory string `json:"-"`
	} `json:"build_materials"`
	PassiveRecipes   []string `json:"passive_recipes"`
	BasedOn          string   `json:"based_on"`
	NPCRole          string   `json:"npc_role"`
	Special          string   `json:"special"`
	PilotingRequired int      `json:"piloting_required"`
	RequiredReputation int    `json:"required_reputation"`
	TowSpeedBonus    int      `json:"tow_speed_bonus"`
	InherentCapabilities []ShipCapability `json:"inherent_capabilities"`
	BoM              *bom.BoMResult
}

// ShipCapability is a built-in ability granted by a ship class.
type ShipCapability struct {
	Type  string `json:"type"`
	Value int    `json:"value"`
	Flag  string `json:"flag"`
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
	return append(catalog.Items, loadLegacyShips()...), nil
}

// legacyShipsOverlay holds hulls the game still flies but no longer sells. The
// catalog is the generator's only ship source, so a retired hull would simply
// have no page — the same reason Nanofiber Internal Structure had none. The
// overlay carries catalog-shaped records rebuilt from the knowledge DB by
// scripts/build_legacy.py, filed under a Discontinued category (their real one
// is blank, since that field only ever came from the catalog they have left).
const legacyShipsOverlay = "overlays/generated/legacy_ships.json"

func loadLegacyShips() []*Ship {
	data, err := os.ReadFile(legacyShipsOverlay)
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "legacy ships overlay: %v\n", err)
		}
		return nil
	}
	var ships []*Ship
	if err := json.Unmarshal(data, &ships); err != nil {
		fmt.Fprintf(os.Stderr, "legacy ships overlay: %v\n", err)
		return nil
	}
	return ships
}

// footprintSrc holds one top-down outline per ship, named by the id the hy3d
// sweep ran under.
const footprintSrc = "data/footprints/hy3d-svg"

// shipFootprint returns the footprint stem to embed for a ship, or "" if it has
// none. The sweep ran before the 2026-03-03 faction-prefix rename, so 32 of the
// discontinued hulls have their outline filed under a retired id -- Benefit's is
// nebula_benefit.svg. Aliases are the only way to find those, which is why the
// pages reference a footprint by source stem rather than by page id: this is
// then the single place that mapping exists, and the staging script stays a
// plain copy.
func shipFootprint(id string, have map[string]bool, legacy kblegacy.Set) string {
	if have[id] {
		return id
	}
	if e, ok := legacy.Ship(id); ok {
		for _, alias := range e.Aliases {
			if have[alias] {
				return alias
			}
		}
	}
	return ""
}

func writeShipPages(outDir string, ships []*Ship, recipeNames map[string]string, items map[string]*Item) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	// Registry blueprint sheets are generated separately (SVG text, see
	// data/footprints/blueprints/make_blueprints.py); embed one on each
	// detail page whose ship has a sheet on disk.
	blueprints := make(map[string]bool)
	if matches, err := filepath.Glob(filepath.Join(outDir, "blueprints", "*.svg")); err == nil {
		for _, m := range matches {
			blueprints[strings.TrimSuffix(filepath.Base(m), ".svg")] = true
		}
	}

	// Hy3d footprint outlines, the fallback for the 109 hulls with no registry
	// blueprint. Read from the committed source rather than the staged copy so
	// generation does not depend on scripts/build-ship-footprints.sh having run.
	footprints := make(map[string]bool)
	if matches, err := filepath.Glob(filepath.Join(footprintSrc, "*.svg")); err == nil {
		for _, m := range matches {
			footprints[strings.TrimSuffix(filepath.Base(m), ".svg")] = true
		}
	}

	// Populate item categories for build materials.
	for _, ship := range ships {
		for i := range ship.BuildMaterials {
			if item, ok := items[ship.BuildMaterials[i].ItemID]; ok {
				ship.BuildMaterials[i].ItemCategory = item.Category
			}
		}
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
		"titleCase": func(s string) string {
			if s == "" {
				return s
			}
			words := strings.Split(s, "_")
			for i, w := range words {
				if w != "" {
					words[i] = strings.ToUpper(w[:1]) + w[1:]
				}
			}
			return strings.Join(words, " ")
		},
		"hasDescription": func(s *Ship) bool { return s.Description != "" },
		"hasLore": func(s *Ship) bool { return s.Lore != "" },
		"hasFlavorTags": func(s *Ship) bool { return len(s.FlavorTags) > 0 },
		"hasBuildMaterials": func(s *Ship) bool { return len(s.BuildMaterials) > 0 },
		"hasDefaultModules": func(s *Ship) bool { return len(s.DefaultModules) > 0 },
		"hasCapabilities": func(s *Ship) bool { return len(s.InherentCapabilities) > 0 },
		"hasRequirements": func(s *Ship) bool { return s.PilotingRequired > 0 || s.RequiredReputation > 0 },
		"hasBoM": func(b *bom.BoMResult) bool {
			return b != nil && len(b.BaseMaterials) > 0
		},
		"boMJSON": func(b *bom.BoMResult) string { return b.JSON() },
		"boMTable": func(b *bom.BoMResult) htmltpl.HTML {
			if b == nil || len(b.BaseMaterials) == 0 {
				return ""
			}

			var sb strings.Builder
			sb.WriteString(`<div class="card" style="padding:0">`)
			sb.WriteString(`<div class="section-label">Construction</div>`)
			sb.WriteString(`<div class="bom-summary-table">`)
			sb.WriteString(`<table><thead><tr><th>Base Material</th><th>Quantity</th></tr></thead><tbody>`)

			for _, mat := range b.BaseMaterials {
				item, ok := items[mat.ItemID]
				if !ok {
					sb.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%d</td></tr>`, mat.ItemID, mat.Quantity))
					continue
				}

				if item.Category == "" {
					sb.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%d</td></tr>`, item.Name, mat.Quantity))
					continue
				}

				sb.WriteString(fmt.Sprintf(`<tr><td><a href="../../items/%s/%s.html">%s</a></td><td>%d</td></tr>`,
					item.Category, mat.ItemID, item.Name, mat.Quantity))
			}

			sb.WriteString(`</tbody></table></div>`)
			sb.WriteString(`</div>`)
			return htmltpl.HTML(sb.String())
		},
		"factionBadge": factionBadgeClass,
		"factionDisplayName": factionDisplayName,
		"hasBlueprint": func(id string) bool { return blueprints[id] },
		"shipFootprint": func(id string) string {
			return shipFootprint(id, footprints, legacySet)
		},
		// Retired hulls stay on the site, loudly marked — people still fly them.
		"legacyShip": func(id string) *kblegacy.Entry {
			if e, ok := legacySet.Ship(id); ok {
				return &e
			}
			return nil
		},
	}

	type pageData struct {
		Categories    []shipCategory
		TotalShips    int
		TotalCats     int
		TotalFactions int
		// TotalBlueprints counts the sheets actually on disk, so the gallery
		// link advertises real coverage rather than the fleet size (not every
		// ship has a hull survey).
		TotalBlueprints int
	}

	tmpl := htmltpl.Must(htmltpl.New("ships").Funcs(funcs).Parse(shipPageTemplate))
	if err := writeTemplate(filepath.Join(outDir, "index.html"), tmpl, pageData{
		Categories:      categories,
		TotalShips:      totalShips,
		TotalCats:       totalCats,
		TotalFactions:   totalFactions,
		TotalBlueprints: len(blueprints),
	}); err != nil {
		return err
	}

	// Write the flat, fully-sortable all-ships table page.
	allShips := slices.Clone(ships)
	slices.SortFunc(allShips, func(a, b *Ship) int { return cmp.Compare(a.Name, b.Name) })
	type allPageData struct {
		Ships      []*Ship
		TotalShips int
	}
	allTmpl := htmltpl.Must(htmltpl.New("ships-all").Funcs(funcs).Parse(shipAllTemplate))
	if err := writeTemplate(filepath.Join(outDir, "all.html"), allTmpl, allPageData{
		Ships:      allShips,
		TotalShips: totalShips,
	}); err != nil {
		return err
	}

	// Write individual ship detail pages.
	detailTmpl := htmltpl.Must(htmltpl.New("ship-detail").Funcs(funcs).Parse(shipDetailTemplate))
	for _, ship := range ships {
		shipDir := filepath.Join(outDir, ship.Category)
		if err := os.MkdirAll(shipDir, 0o755); err != nil {
			return err
		}
		shipPath := filepath.Join(shipDir, ship.ID+".html")
		// Wrap ship with items for template functions
		type shipDetailData struct {
			Ship  *Ship
			Items map[string]*Item
		}
		if err := writeTemplate(shipPath, detailTmpl, shipDetailData{
			Ship:  ship,
			Items: items,
		}); err != nil {
			return err
		}
	}

	return nil
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
      <p class="mt-2"><a href="all.html">&#x25A4; All Ships &mdash; sortable comparison table &rarr;</a></p>
{{- if .TotalBlueprints}}
      <p class="mt-1"><a href="blueprints/index.html">&#x25F1; Registry Blueprints &mdash; three-view drawings for {{.TotalBlueprints}} ships &rarr;</a></p>
      <p class="mt-1"><a href="fitting.html">&#x25E9; Fitting Sheet &mdash; fit modules and read the combined ship stats &rarr;</a></p>
{{- end}}

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
              <td><a href="{{.Category}}/{{.ID}}.html">{{.Name}}</a>{{if legacyShip .ID}} <span class="badge badge-legacy" title="Discontinued — removed from the buyable catalog{{with legacyShip .ID}}{{if .Date}}, last listed {{.Date}}{{end}}{{end}}">Discontinued</span>{{end}}{{if hasPassive .}} <span class="badge badge-green" title="Passive Recipes: {{range $i, $r := .PassiveRecipes}}{{if $i}}, {{end}}{{recipeName $r}}{{end}}">&#x2692; Passive</span>{{end}}</td>
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

var shipAllTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>All Ships - Spacemolt KB</title>
    <link rel="stylesheet" href="../smui.css">
    <link rel="stylesheet" href="ships.css">
</head>
<body>
` + siteHeader + `
    <main class="container page-content ships-all-page">
      <div class="breadcrumb"><a href="index.html">Ships</a> / All Ships</div>
      <h2>All Ships</h2>
      <p class="text-muted mt-1">All {{.TotalShips}} ship classes in one table. Click any column header to sort &mdash; click again to reverse.</p>

      <div class="all-ships-table">
        <table class="sortable">
          <thead>
            <tr>
              <th class="sortable">Name</th>
              <th class="sortable">Category</th>
              <th class="sortable">Class</th>
              <th class="sortable">Faction</th>
              <th class="sortable">Tier</th>
              <th class="sortable">Hull</th>
              <th class="sortable">Armor</th>
              <th class="sortable">Shield</th>
              <th class="sortable">Shd Regen/t</th>
              <th class="sortable">Speed</th>
              <th class="sortable">Fuel</th>
              <th class="sortable">Cargo</th>
              <th class="sortable">Wpn</th>
              <th class="sortable">Def</th>
              <th class="sortable">Util</th>
              <th class="sortable">Power</th>
              <th class="sortable">CPU</th>
              <th class="sortable">Price</th>
              <th class="sortable">Build</th>
              <th class="sortable">Yard</th>
              <th class="sortable">Pilot</th>
            </tr>
          </thead>
          <tbody>
{{- range .Ships}}
            <tr>
              <td><a href="{{.Category}}/{{.ID}}.html">{{.Name}}</a>{{if hasPassive .}} <span class="badge badge-green" title="Passive Recipes: {{range $i, $r := .PassiveRecipes}}{{if $i}}, {{end}}{{recipeName $r}}{{end}}">&#x2692;</span>{{end}}</td>
              <td{{if legacyShip .ID}} class="cat-legacy" title="Removed from the buyable catalog{{with legacyShip .ID}}{{if .Date}}, last listed {{.Date}}{{end}}{{end}}"{{end}}>{{.Category}}</td>
              <td>{{.Class}}</td>
              <td data-sort="{{factionDisplayName .Faction}}"><span class="badge {{factionBadge .Faction}}">{{factionDisplayName .Faction}}</span></td>
              <td class="num">{{.Tier}}</td>
              <td class="num">{{.BaseHull}}</td>
              <td class="num">{{.BaseArmor}}</td>
              <td class="num">{{.BaseShield}}</td>
              <td class="num">{{.BaseShieldRecharge}}</td>
              <td class="num">{{.BaseSpeed}}</td>
              <td class="num">{{.BaseFuel}}</td>
              <td class="num">{{.CargoCapacity}}</td>
              <td class="num">{{.WeaponSlots}}</td>
              <td class="num">{{.DefenseSlots}}</td>
              <td class="num">{{.UtilitySlots}}</td>
              <td class="num">{{.PowerCapacity}}</td>
              <td class="num">{{.CPUCapacity}}</td>
              <td class="num">{{.Price}}</td>
              <td class="num">{{.BuildTime}}</td>
              <td class="num">{{.ShipyardTier}}</td>
              <td class="num">{{.PilotingRequired}}</td>
            </tr>
{{- end}}
          </tbody>
        </table>
      </div>
    </main>
` + sortScript + themeScript + `
</body>
</html>
`

var shipDetailTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Ship.Name}} - Ships - Spacemolt KB</title>
    <link rel="stylesheet" href="../../smui.css">
    <link rel="stylesheet" href="../ships.css">
</head>
<body>
` + siteHeaderSub + `
    <main class="container page-content">
{{- with .Ship}}
        <div class="breadcrumb"><a href="../">Ships</a> / <a href="./">{{.Category}}</a> / {{.Name}}</div>
        <h2>{{.Name}}</h2>
{{- with legacyShip .ID}}
        <div class="legacy-note">
          <span class="badge badge-legacy">Discontinued</span>
          Removed from the buyable catalog{{if .Date}} — last listed on {{.Date}}{{end}}.
          It cannot be ordered, but shipyards can still build and list these,
          so one may still turn up for sale. Existing hulls fly and fit normally.
{{- if .Aliases}} Also known as {{range $i, $a := .Aliases}}{{if $i}}, {{end}}<code>{{$a}}</code>{{end}}.{{end}}
        </div>
{{- end}}

{{- if hasDescription .}}
        <blockquote class="item-desc">{{.Description}}</blockquote>
{{- end}}

{{- if hasBlueprint .ID}}
        <div class="card mt-2 blueprint-card">
          <div class="section-label">Registry Blueprint <a class="blueprint-open" href="../blueprints/{{.ID}}.svg" title="Open full-size blueprint">&#x2197; full size</a></div>
          {{/* object, not img: the sheet references its perspective-view
               raster (blueprints/art/) and img contexts block external
               loads inside SVG */}}
          <object class="blueprint" data="../blueprints/{{.ID}}.svg" type="image/svg+xml" aria-label="{{.Name}} registry blueprint"></object>
        </div>
{{- else if shipFootprint .ID}}
        {{/* No registry blueprint drawn for this hull, but the hy3d sweep
             traced its outline. For 32 discontinued hulls that outline is
             filed under a retired id, which is why the href is the resolved
             stem rather than .ID. */}}
        <div class="card mt-2 blueprint-card">
          <div class="section-label">Footprint <a class="blueprint-open" href="../footprints/{{shipFootprint .ID}}.svg" title="Open full-size footprint">&#x2197; full size</a></div>
          <object class="blueprint footprint" data="../footprints/{{shipFootprint .ID}}.svg" type="image/svg+xml" aria-label="{{.Name}} top-down hull outline"></object>
          <p class="text-muted mt-1">Top-down hull outline traced from the ship model. No registry blueprint has been drawn for this hull.</p>
        </div>
{{- end}}

        <div class="card mt-2" style="padding:0">
          <div class="section-label">General</div>
          <table>
            <tr><td class="kv-label">Category</td><td><a href="./">{{.Category}}</a></td></tr>
            <tr><td class="kv-label">Class</td><td><a href="../index.html#{{slugify .Category}}--{{slugify .Class}}">{{.Class}}</a></td></tr>
{{- if .Faction}}
            <tr><td class="kv-label">Faction</td><td><span class="badge {{factionBadge .Faction}}">{{factionDisplayName .Faction}}</span></td></tr>
{{- end}}
            <tr><td class="kv-label">Tier</td><td>{{.Tier}}</td></tr>
{{- if .Special}}
            <tr><td class="kv-label">Special</td><td>{{titleCase .Special}}</td></tr>
{{- end}}
{{- if .NPCRole}}
            <tr><td class="kv-label">NPC Role</td><td>{{titleCase .NPCRole}}</td></tr>
{{- end}}
{{- if .BasedOn}}
            <tr><td class="kv-label">Based On</td><td>{{titleCase .BasedOn}}</td></tr>
{{- end}}
{{- if .StarterShip}}
            <tr><td class="kv-label">Type</td><td><span class="badge badge-green">Starter Ship</span></td></tr>
{{- end}}
          </table>

{{- if hasRequirements .}}
          <div class="section-label">Requirements</div>
          <table>
{{- if gt .PilotingRequired 0}}
            <tr><td class="kv-label">Piloting Skill</td><td>{{.PilotingRequired}}</td></tr>
{{- end}}
{{- if gt .RequiredReputation 0}}
            <tr><td class="kv-label">Reputation</td><td>{{.RequiredReputation}}</td></tr>
{{- end}}
          </table>
{{- end}}

          <div class="section-label">Statistics</div>
          <table>
            <tr><td class="kv-label">Hull</td><td>{{.BaseHull}}</td></tr>
            <tr><td class="kv-label">Armor</td><td>{{.BaseArmor}}</td></tr>
            <tr><td class="kv-label">Shield</td><td>{{.BaseShield}}</td></tr>
            <tr><td class="kv-label">Shield Recharge</td><td>{{.BaseShieldRecharge}}/t</td></tr>
            <tr><td class="kv-label">Speed</td><td>{{.BaseSpeed}} AU/t</td></tr>
            <tr><td class="kv-label">Fuel Capacity</td><td>{{.BaseFuel}}</td></tr>
            <tr><td class="kv-label">Cargo Capacity</td><td>{{.CargoCapacity}}</td></tr>
{{- if gt .TowSpeedBonus 0}}
            <tr><td class="kv-label">Tow Speed Bonus</td><td>+{{.TowSpeedBonus}}%</td></tr>
{{- end}}
          </table>

          <div class="section-label">Slots</div>
          <table>
            <tr><td class="kv-label">Weapon Slots</td><td>{{.WeaponSlots}}</td></tr>
            <tr><td class="kv-label">Defense Slots</td><td>{{.DefenseSlots}}</td></tr>
            <tr><td class="kv-label">Utility Slots</td><td>{{.UtilitySlots}}</td></tr>
          </table>

          <div class="section-label">Capacity</div>
          <table>
            <tr><td class="kv-label">Power Capacity</td><td>{{.PowerCapacity}}</td></tr>
            <tr><td class="kv-label">CPU Capacity</td><td>{{.CPUCapacity}}</td></tr>
          </table>

          <div class="section-label">Economy</div>
          <table>
            <tr><td class="kv-label">Price</td><td>{{if gt .Price 0}}{{.Price}} cr{{else}}Free{{end}}</td></tr>
            <tr><td class="kv-label">Build Time</td><td>{{.BuildTime}} ticks</td></tr>
            <tr><td class="kv-label">Shipyard Tier</td><td>{{.ShipyardTier}}</td></tr>
          </table>
        </div>

{{- if hasLore .}}
        <div class="card mt-2" style="padding:0">
          <div class="section-label">Lore</div>
          <div class="ship-lore">{{.Lore}}</div>
        </div>
{{- end}}

{{- if hasFlavorTags .}}
        <div class="card mt-2" style="padding:0">
          <div class="section-label">Flavor Tags</div>
          <div class="flavor-tags">
{{- range .FlavorTags}}
            <span class="badge badge-frost">{{titleCase .}}</span>
{{- end}}
          </div>
        </div>
{{- end}}

{{- if hasDefaultModules .}}
        <div class="card mt-2" style="padding:0">
          <div class="section-label">Default Modules</div>
          <table>
            <thead><tr><th>Module</th></tr></thead>
            <tbody>
{{- range .DefaultModules}}
{{- $mod := index $.Items .}}
            <tr>
{{- if $mod}}
              <td><a href="../../items/{{$mod.Category}}/{{.}}.html">{{$mod.Name}}</a></td>
{{- else}}
              <td>{{titleCase .}}</td>
{{- end}}
            </tr>
{{- end}}
            </tbody>
          </table>
        </div>
{{- end}}

{{- if hasCapabilities .}}
        <div class="card mt-2" style="padding:0">
          <div class="section-label">Inherent Capabilities</div>
          <table>
            <thead><tr><th>Capability</th><th>Value</th></tr></thead>
            <tbody>
{{- range .InherentCapabilities}}
            <tr>
              <td>{{titleCase .Type}}{{if .Flag}} <span class="badge badge-frost">{{titleCase .Flag}}</span>{{end}}</td>
              <td class="num">{{if gt .Value 0}}{{.Value}}{{else}}&#x2713;{{end}}</td>
            </tr>
{{- end}}
            </tbody>
          </table>
        </div>
{{- end}}

{{- if hasBuildMaterials .}}
        <div class="card mt-2" style="padding:0">
          <div class="section-label">Build Materials</div>
          <table>
            <thead><tr><th>Item</th><th>Quantity</th></tr></thead>
            <tbody>
{{- range .BuildMaterials}}
            <tr>
              <td><a href="../../items/{{.ItemCategory}}/{{.ItemID}}.html">{{titleCase .ItemID}}</a></td>
              <td>{{.Quantity}}</td>
            </tr>
{{- end}}
            </tbody>
          </table>
        </div>
{{- end}}

{{- if hasPassive .}}
        <div class="card mt-2" style="padding:0">
          <div class="section-label">Passive Recipes</div>
          <table>
            <thead><tr><th>Recipe</th></tr></thead>
            <tbody>
{{- range .PassiveRecipes}}
            <tr>
              <td><a href="../../recipes/Legendary/{{.}}.html">{{recipeName .}}</a></td>
            </tr>
{{- end}}
            </tbody>
          </table>
        </div>
{{- end}}

{{- if hasBoM .BoM}}
        {{boMTable .BoM}}
        <details class="bom-json-details">
          <summary>View JSON Data</summary>
          <pre class="bom-json">{{boMJSON .BoM}}</pre>
        </details>
{{- end}}
{{- end}}

    </main>
` + themeScript + `
</body>
</html>
`
