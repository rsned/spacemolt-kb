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
			p.hidden
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
			&e.Richness, &e.Remaining, &e.LastUpdatedTick, &e.Hidden,
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

func writeResourcePages(outDir string, db *sql.DB) error {
	entries, err := loadResourceEntries(db)
	if err != nil {
		return fmt.Errorf("load resource entries: %w", err)
	}

	// Group by resource name.
	groupMap := make(map[string]*ResourceGroup)
	var groupOrder []string
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
		"anchorID": func(name string) string {
			r := strings.NewReplacer(" ", "-", "'", "")
			return strings.ToLower(r.Replace(name))
		},
		"sanitizeName": sanitizeName,
		"itemPageURL": func(category, resourceID string) string {
			if category == "" {
				return ""
			}
			return fmt.Sprintf("../items/%s/%s.html", category, resourceID)
		},
	}

	tmpl := htmltpl.Must(htmltpl.New("resources").Funcs(funcs).Parse(resourceIndexTemplate))

	data := struct {
		Groups     []ResourceGroup
		TotalPOIs  int
		TotalTypes int
	}{
		Groups:     groups,
		TotalPOIs:  len(entries),
		TotalTypes: len(groups),
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
        @media (max-width: 768px) { .toc { columns: 2; } }
        @media (max-width: 480px) { .toc { columns: 1; } }
    </style>
</head>
<body>
` + siteHeader + `
    <main class="container page-content">
        <h2>Resources</h2>
        <p>All known mineable resources across surveyed systems, grouped by type.</p>

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

        <div class="card" style="padding: 12px 16px">
            <div class="section-label">Jump To Resource</div>
            <div class="toc">
{{- range .Groups}}
                <a href="#{{anchorID .ResourceName}}">{{.ResourceName}} ({{len .Entries}})</a>
{{- end}}
            </div>
        </div>

{{- range .Groups}}
        <div id="{{anchorID .ResourceName}}" class="resource-section">
            <h3>{{.ResourceName}} <span class="badge" style="font-size:0.7em; vertical-align:middle;">{{len .Entries}} deposits</span>{{if .ResourceCategory}} <small style="font-size:0.8em; font-weight:normal;"><a href="{{itemPageURL .ResourceCategory .ResourceID}}">Details</a></small>{{end}} <a href="#" class="back-top">[top]</a></h3>
            <table class="sortable">
                <thead>
                    <tr>
                        <th>System</th>
                        <th>System ID</th>
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
        </div>
{{- end}}
    </main>
` + sortScript + `
` + themeScript + `
</body>
</html>
`
