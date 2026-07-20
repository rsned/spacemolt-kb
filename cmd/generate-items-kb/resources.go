package main

import (
	"cmp"
	"database/sql"
	"fmt"
	htmltpl "html/template"
	"log"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"

	humanize "github.com/dustin/go-humanize"

	"github.com/rsned/spacemolt-kb/pkg/galaxymap"
)

// ResourceEntry is a single resource occurrence at a POI.
type ResourceEntry struct {
	SystemName      string
	SystemID        string
	POIName         string
	POIID           string
	ResourceName    string
	ResourceID      string
	ResourceCategory string
	Richness        float64
	MaxAmount       float64
	Remaining       float64
	DepletionPct    float64 // 0–100, how much has been consumed
	LastUpdatedTick int
	Hidden          bool
	StationInSystem bool
}

// ResourceGroup groups all occurrences of a single resource.
type ResourceGroup struct {
	ResourceName     string
	ResourceID       string
	ResourceCategory string
	Entries          []ResourceEntry
}

func loadResourceEntries(db *sql.DB) ([]ResourceEntry, error) {
	rows, err := db.Query(`
		SELECT
			s.name, s.id,
			p.name, p.id,
			COALESCE(i.name, pr.resource_id), pr.resource_id,
			COALESCE(i.category, ''),
			pr.richness, pr.remaining, pr.last_updated_tick,
			p.hidden,
			EXISTS(SELECT 1 FROM pois sp WHERE sp.system_id = s.id AND sp.type = 'station')
		FROM poi_resources pr
		JOIN pois p ON pr.poi_id = p.id
		JOIN systems s ON p.system_id = s.id
		LEFT JOIN items i ON pr.resource_id = i.id
		ORDER BY COALESCE(i.name, pr.resource_id), s.name, p.name
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	// First pass: collect entries and track max remaining per (poi, resource).
	var entries []ResourceEntry
	for rows.Next() {
		var e ResourceEntry
		if err := rows.Scan(
			&e.SystemName, &e.SystemID,
			&e.POIName, &e.POIID,
			&e.ResourceName, &e.ResourceID, &e.ResourceCategory,
			&e.Richness, &e.Remaining, &e.LastUpdatedTick, &e.Hidden, &e.StationInSystem,
		); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Compute max amount per resource across all POIs (highest observed remaining).
	maxByResource := make(map[string]float64)
	for _, e := range entries {
		if e.Remaining > maxByResource[e.ResourceID] {
			maxByResource[e.ResourceID] = e.Remaining
		}
	}

	// Set max amount and depletion percent.
	for i := range entries {
		e := &entries[i]
		e.MaxAmount = maxByResource[e.ResourceID]
		if e.MaxAmount > 0 {
			e.DepletionPct = math.Round((1 - e.Remaining/e.MaxAmount) * 100)
		}
	}

	return entries, nil
}

// loadSystemsForStats loads minimal system data for exploration statistics.
func loadSystemsForStats(db *sql.DB) ([]struct {
	ID              string
	LastUpdatedTick int
}, error) {
	rows, err := db.Query(`SELECT id, last_updated_tick FROM systems`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var systems []struct {
		ID              string
		LastUpdatedTick int
	}
	for rows.Next() {
		var s struct {
			ID              string
			LastUpdatedTick int
		}
		if err := rows.Scan(&s.ID, &s.LastUpdatedTick); err != nil {
			return nil, err
		}
		systems = append(systems, s)
	}
	return systems, rows.Err()
}

// loadAllResourceItems loads all items from ore and material categories that should appear as resources,
// even if they haven't been discovered yet.
func loadAllResourceItems(db *sql.DB) (map[string]struct {
	Name     string
	Category string
}, error) {
	rows, err := db.Query(`
		SELECT id, COALESCE(name, id), COALESCE(category, '')
		FROM items
		WHERE category IN ('ore', 'material')
		ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make(map[string]struct {
		Name     string
		Category string
	})
	for rows.Next() {
		var id, name, category string
		if err := rows.Scan(&id, &name, &category); err != nil {
			return nil, err
		}
		items[id] = struct {
			Name     string
			Category string
		}{Name: name, Category: category}
	}
	return items, rows.Err()
}

// resourceSlug converts a resource display name into the slug shared by the
// page anchor, the highlight CSS class, and the map dropdown value.
func resourceSlug(name string) string {
	r := strings.NewReplacer(" ", "-", "'", "")
	return strings.ToLower(r.Replace(name))
}

// systemResourceClasses maps each system ID to the sorted set of
// "r-<slug>" classes for the resources it contains. Systems with no
// surveyed deposits are absent from the result.
func systemResourceClasses(groups []ResourceGroup) map[string][]string {
	seen := make(map[string]map[string]bool)
	for _, g := range groups {
		if len(g.Entries) == 0 {
			continue
		}
		class := "r-" + resourceSlug(g.ResourceName)
		for _, e := range g.Entries {
			if seen[e.SystemID] == nil {
				seen[e.SystemID] = make(map[string]bool)
			}
			seen[e.SystemID][class] = true
		}
	}

	out := make(map[string][]string, len(seen))
	for sysID, classSet := range seen {
		classes := make([]string, 0, len(classSet))
		for c := range classSet {
			classes = append(classes, c)
		}
		slices.Sort(classes)
		out[sysID] = classes
	}
	return out
}

// resourceHighlightCSS emits one highlight rule per discovered resource.
func resourceHighlightCSS(groups []ResourceGroup) string {
	var b strings.Builder
	for _, g := range groups {
		if len(g.Entries) == 0 {
			continue
		}
		slug := resourceSlug(g.ResourceName)
		fmt.Fprintf(&b, "#res-map[data-active=\"%s\"] .r-%s{fill:#ffcc44;r:6;stroke:#7a5c00}\n", slug, slug)
	}
	return b.String()
}

// loadSystemsForMap loads system geometry and jump connections for the
// resource map. Unlike loadSystemsForStats it pulls position and empire.
func loadSystemsForMap(db *sql.DB) ([]*galaxymap.System, map[string]*galaxymap.System, error) {
	rows, err := db.Query(`
		SELECT id, name, position_x, position_y, police_level,
		       COALESCE(empire, ''), is_stronghold, last_updated_tick
		FROM systems ORDER BY name
	`)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()

	var systems []*galaxymap.System
	byID := make(map[string]*galaxymap.System)
	for rows.Next() {
		var s galaxymap.System
		if err := rows.Scan(&s.ID, &s.Name, &s.PositionX, &s.PositionY,
			&s.PoliceLevel, &s.Empire, &s.IsStronghold, &s.LastUpdatedTick); err != nil {
			return nil, nil, err
		}
		if s.ID == "" {
			continue
		}
		systems = append(systems, &s)
		byID[s.ID] = &s
	}
	return systems, byID, rows.Err()
}

func writeResourcePages(outDir string, db *sql.DB) error {
	entries, err := loadResourceEntries(db)
	if err != nil {
		return fmt.Errorf("load resource entries: %w", err)
	}

	// Load all ore and material items to include undiscovered ones.
	allResourceItems, err := loadAllResourceItems(db)
	if err != nil {
		return fmt.Errorf("load all resource items: %w", err)
	}

	// Group by resource name, including undiscovered items.
	groupMap := make(map[string]*ResourceGroup)
	var groupOrder []string

	// First, add all discovered resources.
	for _, e := range entries {
		g, ok := groupMap[e.ResourceID]
		if !ok {
			g = &ResourceGroup{
				ResourceName:     e.ResourceName,
				ResourceID:       e.ResourceID,
				ResourceCategory: e.ResourceCategory,
			}
			groupMap[e.ResourceID] = g
			groupOrder = append(groupOrder, e.ResourceID)
		}
		g.Entries = append(g.Entries, e)
	}

	// Then, add undiscovered resources (ore and material items with no entries).
	for id, item := range allResourceItems {
		if _, exists := groupMap[id]; !exists {
			groupMap[id] = &ResourceGroup{
				ResourceName:     item.Name,
				ResourceID:       id,
				ResourceCategory: item.Category,
				Entries:          []ResourceEntry{}, // Empty entries = undiscovered
			}
			groupOrder = append(groupOrder, id)
		}
	}

	groups := make([]ResourceGroup, 0, len(groupMap))
	for _, id := range groupOrder {
		groups = append(groups, *groupMap[id])
	}
	slices.SortFunc(groups, func(a, b ResourceGroup) int {
		return cmp.Compare(a.ResourceName, b.ResourceName)
	})

	// Clean old HTML files.
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	dirEntries, _ := os.ReadDir(outDir)
	for _, e := range dirEntries {
		if strings.HasSuffix(e.Name(), ".html") {
			_ = os.Remove(filepath.Join(outDir, e.Name()))
		}
	}

	funcs := htmltpl.FuncMap{
		"fmtNum": func(f float64) string {
			return humanize.Comma(int64(f))
		},
		"fmtRichness": func(f float64) string {
			return fmt.Sprintf("%.0f", f)
		},
		"fmtDepletion": func(f float64) string {
			return fmt.Sprintf("%.0f%%", f)
		},
		"depletionClass": func(f float64) string {
			switch {
			case f >= 100:
				return "depleted"
			case f >= 75:
				return "high"
			case f >= 25:
				return "medium"
			default:
				return "low"
			}
		},
		"fmtTick": func(t int) string {
			if t < 400000 {
				return "-"
			}
			return fmt.Sprintf("%d", t)
		},
		"anchorID": resourceSlug,
		"sanitizeName": sanitizeName,
		"itemPageURL": func(category, resourceID string) string {
			if category == "" {
				return ""
			}
			return fmt.Sprintf("../items/%s/%s.html", category, resourceID)
		},
	}

	tmpl := htmltpl.Must(htmltpl.New("resources").Funcs(funcs).Parse(resourceIndexTemplate))

	// Load systems to calculate exploration statistics.
	systems, err := loadSystemsForStats(db)
	if err != nil {
		return fmt.Errorf("load systems for stats: %w", err)
	}
	totalSystems := len(systems)
	exploredSystems := 0
	for _, s := range systems {
		if s.LastUpdatedTick > 0 {
			exploredSystems++
		}
	}
	explorationPct := 0.0
	if totalSystems > 0 {
		explorationPct = 100.0 * float64(exploredSystems) / float64(totalSystems)
	}

	mapSystems, mapByID, err := loadSystemsForMap(db)
	if err != nil {
		return fmt.Errorf("load systems for map: %w", err)
	}
	classes := systemResourceClasses(groups)

	var explored, unexplored []*galaxymap.System
	for _, s := range mapSystems {
		if s.LastUpdatedTick > 0 {
			explored = append(explored, s)
		} else {
			unexplored = append(unexplored, s)
		}
	}

	mapSVG := galaxymap.Render(explored, unexplored, mapByID, galaxymap.Options{
		ShowEmpireBlobs:  false,
		ShowConnections:  false,
		LinkPrefix:       "../",
		HighlightClasses: func(id string) []string { return classes[id] },
	})

	firstSlug := ""
	for _, g := range groups {
		if len(g.Entries) > 0 {
			firstSlug = resourceSlug(g.ResourceName)
			break
		}
	}

	data := struct {
		Groups          []ResourceGroup
		TotalPOIs       int
		TotalTypes      int
		TotalSystems    int
		ExploredSystems int
		ExplorationPct  float64
		MapSVG          htmltpl.HTML
		HighlightCSS    htmltpl.CSS
		FirstSlug       string
	}{
		Groups:          groups,
		TotalPOIs:       len(entries),
		TotalTypes:      len(groups),
		TotalSystems:    totalSystems,
		ExploredSystems: exploredSystems,
		ExplorationPct:  explorationPct,
		MapSVG:          htmltpl.HTML(mapSVG),         //nolint:gosec // generated internally from trusted DB data
		HighlightCSS:    htmltpl.CSS(resourceHighlightCSS(groups)), //nolint:gosec // generated internally from trusted DB data
		FirstSlug:       firstSlug,
	}

	outPath := filepath.Join(outDir, "index.html")
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	if err := tmpl.Execute(f, data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	log.Printf("Resources: %d types, %d total entries", len(groups), len(entries))
	return nil
}

var resourceIndexTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Resources - Spacemolt KB</title>
    <link rel="stylesheet" href="../smui.css">
    <link rel="stylesheet" href="../system.css">
    <style>
        .toc { columns: 3; column-gap: 24px; margin: 16px 0 32px; }
        .toc a { display: block; padding: 2px 0; color: var(--link); text-decoration: none; font-size: 0.95em; }
        .toc a:hover { text-decoration: underline; }
        .resource-section { margin-top: 32px; scroll-margin-top: 16px; }
        .resource-section h3 { margin-bottom: 8px; border-bottom: 1px solid var(--border); padding-bottom: 4px; }
        .resource-section table { width: 100%; font-size: 0.9em; }
        .resource-section th { text-align: left; cursor: pointer; user-select: none; white-space: nowrap; }
        .resource-section th:hover { color: var(--link); }
        .resource-section td { padding: 4px 8px; }
        .resource-section tr:hover { background: var(--bg-hover, rgba(128,128,128,0.08)); }
        .depletion { font-weight: 600; }
        .depletion.depleted { color: #d04040; }
        .depletion.high { color: #d08040; }
        .depletion.medium { color: #c0a030; }
        .depletion.low { color: #50a050; }
        .back-top { font-size: 0.8em; margin-left: 8px; color: var(--text-muted); }
        .stat-bar { display: inline-block; width: 100%; }
        .summary-cards { display: flex; gap: 16px; margin: 16px 0; flex-wrap: wrap; }
        .summary-card { background: var(--bg-card); border: 1px solid var(--border); border-radius: 8px; padding: 12px 20px; text-align: center; }
        .summary-card .num { font-size: 1.8em; font-weight: 700; }
        .summary-card .label { font-size: 0.8em; color: var(--text-muted); text-transform: uppercase; }
        .undiscovered { background: var(--bg-card); border: 1px solid var(--border); border-left: 4px solid #999; padding: 16px; margin-top: 16px; border-radius: 4px; }
        .undiscovered h4 { margin: 0 0 8px 0; color: #666; font-size: 0.95em; }
        .undiscovered p { margin: 0; color: var(--text-muted); font-size: 0.9em; }
        @media (max-width: 768px) { .toc { columns: 2; } }
        @media (max-width: 480px) { .toc { columns: 1; } }
        #res-map-wrap { position: sticky; top: 8px; float: right; width: 380px;
            margin: 0 0 16px 24px; background: var(--bg-card);
            border: 1px solid var(--border); border-radius: 8px; padding: 12px; }
        #res-map-wrap svg { width: 100%; height: auto; display: block;
            border-radius: 4px; }
        #res-map { width: 100%; max-width: 480px; }
        #res-map-wrap select { width: 100%; margin-top: 8px; padding: 4px; }
        .res-map-empty { display: none; font-size: 0.85em; color: var(--text-muted);
            margin-top: 8px; }
        #res-map[data-empty="1"] + .res-map-empty { display: block; }
        #res-map .galaxy-sys-dot { fill: #2a3038; r: 3; transition: none; }
    {{.HighlightCSS}}
        @media (max-width: 1100px) {
            #res-map-wrap { float: none; width: 100%; position: static; margin-left: 0; }
        }
    </style>
</head>
<body>
` + siteHeader + `
    <main class="container page-content">
        <h2>Resources</h2>
        <p>All known mineable resources across surveyed systems, grouped by type.</p>

        <div id="res-map-wrap">
            <div id="res-map" data-active="{{.FirstSlug}}">{{.MapSVG}}</div>
            <div class="res-map-empty">No systems with this resource have been surveyed yet.</div>
            <select id="res-map-select" aria-label="Highlight resource on map">
{{- range .Groups}}
{{- if gt (len .Entries) 0}}
                <option value="{{anchorID .ResourceName}}">{{.ResourceName}} ({{len .Entries}})</option>
{{- end}}
{{- end}}
            </select>
        </div>

        <div class="summary-cards">
            <div class="summary-card">
                <div class="num">{{.TotalTypes}}</div>
                <div class="label">Resource Types</div>
            </div>
            <div class="summary-card">
                <div class="num">{{.TotalPOIs}}</div>
                <div class="label">Total Deposits</div>
            </div>
        </div>

        <div class="summary-cards">
            <div class="summary-card">
                <div class="num">{{.TotalSystems}}</div>
                <div class="label">Star Systems</div>
            </div>
            <div class="summary-card">
                <div class="num">{{.ExploredSystems}}</div>
                <div class="label">Systems Explored</div>
            </div>
            <div class="summary-card">
                <div class="num">{{printf "%.1f" .ExplorationPct}}%</div>
                <div class="label">Galaxy Explored</div>
            </div>
        </div>

        <div class="card" style="padding: 12px 16px">
            <div class="section-label">Jump To Resource</div>
            <div class="toc">
{{- range .Groups}}
                <a href="#{{anchorID .ResourceName}}">{{.ResourceName}} ({{if eq (len .Entries) 0}}Undiscovered{{else}}{{len .Entries}}{{end}})</a>
{{- end}}
            </div>
        </div>

{{- range .Groups}}
        <div id="{{anchorID .ResourceName}}" class="resource-section">
            <h3>{{.ResourceName}} <span class="badge" style="font-size:0.7em; vertical-align:middle;">{{if eq (len .Entries) 0}}Undiscovered{{else}}{{len .Entries}} deposits{{end}}</span>{{if .ResourceCategory}} <small style="font-size:0.8em; font-weight:normal;"><a href="{{itemPageURL .ResourceCategory .ResourceID}}">Details</a></small>{{end}} <a href="#" class="back-top">[top]</a></h3>
{{- if eq (len .Entries) 0}}
            <div class="undiscovered">
                <h4>Not Yet Discovered</h4>
                <p>This resource has not been found in any surveyed system yet. Exploration agents may discover it in uncharted regions of the galaxy.</p>
            </div>
{{- else}}
            <table class="sortable">
                <thead>
                    <tr>
                        <th>System</th>
                        <th>System ID</th>
                        <th>Station</th>
                        <th>POI</th>
                        <th>POI ID</th>
                        <th>Hidden</th>
                        <th>Resource ID</th>
                        <th>Richness</th>
                        <th>Max Amount</th>
                        <th>Remaining</th>
                        <th>Depletion</th>
                        <th>Last Updated</th>
                    </tr>
                </thead>
                <tbody>
{{- range .Entries}}
                    <tr>
                        <td><a href="../systems/{{.SystemID}}/index.html">{{.SystemName}}</a></td>
                        <td><code>{{.SystemID}}</code></td>
                        <td>{{if .StationInSystem}}<span class="badge badge-green">✓</span>{{else}}<span class="text-muted">—</span>{{end}}</td>
                        <td>{{.POIName}}</td>
                        <td><code>{{.POIID}}</code></td>
                        <td>{{if .Hidden}}<span class="badge badge-yellow">Yes</span>{{else}}<span class="text-muted">—</span>{{end}}</td>
                        <td><code>{{.ResourceID}}</code></td>
                        <td>{{fmtRichness .Richness}}</td>
                        <td>{{fmtNum .MaxAmount}}</td>
                        <td>{{fmtNum .Remaining}}</td>
                        <td><span class="depletion {{depletionClass .DepletionPct}}">{{fmtDepletion .DepletionPct}}</span></td>
                        <td>{{fmtTick .LastUpdatedTick}}</td>
                    </tr>
{{- end}}
                </tbody>
            </table>
{{- end}}
        </div>
{{- end}}
    </main>
` + resMapSyncScript + `
` + sortScript + `
` + themeScript + `
</body>
</html>
`

// resMapSyncScript keeps the resource-map highlight in sync between the
// dropdown, the URL hash, and the map's data-active attribute.
var resMapSyncScript = `    <script>
    (function () {
      var map = document.getElementById('res-map');
      var sel = document.getElementById('res-map-select');
      if (!map || !sel) return;

      var valid = {};
      for (var i = 0; i < sel.options.length; i++) valid[sel.options[i].value] = true;

      function apply(slug, updateHash) {
        if (!valid[slug]) {
          map.setAttribute('data-empty', '1');
          map.removeAttribute('data-active');
          return;
        }
        map.removeAttribute('data-empty');
        map.setAttribute('data-active', slug);
        if (sel.value !== slug) sel.value = slug;
        if (updateHash && location.hash.slice(1) !== slug) {
          history.replaceState(null, '', '#' + slug);
        }
      }

      sel.addEventListener('change', function () { apply(sel.value, true); });
      window.addEventListener('hashchange', function () { apply(location.hash.slice(1), false); });

      var initial = location.hash.slice(1);
      apply(valid[initial] ? initial : sel.value, false);
    })();
    </script>`
