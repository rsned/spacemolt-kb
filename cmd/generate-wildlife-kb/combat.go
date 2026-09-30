package main

import (
	"cmp"
	htmltpl "html/template"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ratingRank orders the public-feed danger tiers for sorting; unrated species
// (too few battles) sort below "minimal".
var ratingRank = map[string]int{"extreme": 5, "high": 4, "moderate": 3, "low": 2, "minimal": 1}

// combatRow is one species on the combat summary page. Numeric fields use -1
// for "no data" so a column sort puts unknowns at one end, never mixed in.
type combatRow struct {
	ID, Name, Role string
	Hull, Shield   int
	Rating         string
	RatingRank     int
	Battles        int     // public battle feed
	WinPct         float64 // wildlife side's win rate, -1 = no battles
	DamageTypes    []string
	DmgMin, DmgMax int // base damage per shot across its weapons, -1 = never seen attacking
	Shots          int
	HitMin, HitMax float64 // -1 = unknown
	LogBattles     int     // exported battle logs behind the damage/hit columns
	Kills          int
}

func buildCombatRows(views []speciesView) []combatRow {
	rows := make([]combatRow, 0, len(views))
	for _, v := range views {
		r := combatRow{ID: v.ID, Name: v.Name, Role: v.Role, Hull: v.MaxHull, Shield: v.MaxShield,
			WinPct: -1, DmgMin: -1, DmgMax: -1, HitMin: -1, HitMax: -1, Kills: len(v.Kills)}
		if v.Record != nil && v.Record.Battles > 0 {
			r.Battles = v.Record.Battles
			r.WinPct = v.Record.WinPct()
			r.Rating = v.Record.Rating()
			r.RatingRank = ratingRank[r.Rating]
		}
		if c := v.Combat; c != nil {
			r.LogBattles = c.Battles
			r.DamageTypes = c.DamageTypes()
			for _, w := range c.Weapons {
				if r.DmgMin < 0 || w.BaseMin < r.DmgMin {
					r.DmgMin = w.BaseMin
				}
				r.DmgMax = max(r.DmgMax, w.BaseMax)
				r.Shots += w.Shots
			}
			if c.HitMax > 0 {
				r.HitMin, r.HitMax = c.HitMin, c.HitMax
			}
		}
		rows = append(rows, r)
	}
	// Most dangerous first: feed rating, then win rate, then hardest hitter.
	slices.SortFunc(rows, func(a, b combatRow) int {
		return cmp.Or(cmp.Compare(b.RatingRank, a.RatingRank), cmp.Compare(b.WinPct, a.WinPct),
			cmp.Compare(b.DmgMax, a.DmgMax), strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)))
	})
	return rows
}

func writeCombatPage(outDir string, header htmltpl.HTML, views []speciesView, statsMonths string) error {
	t := htmltpl.Must(htmltpl.New("combat").Funcs(templateFuncs()).Parse(combatTemplate))
	f, err := os.Create(filepath.Join(outDir, "combat.html"))
	if err != nil {
		return err
	}
	err = t.Execute(f, map[string]any{
		"Header":      header,
		"Rows":        buildCombatRows(views),
		"StatsMonths": statsMonths,
	})
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// combatSortScript: click a th.sortable to sort by it (again to reverse);
// cells sort by data-sort when present, numerically when both parse.
const combatSortScript = `    <script>
    document.querySelectorAll("table.sortable").forEach(function (table) {
      var sortCol = -1, sortAsc = true;
      table.querySelectorAll("th.sortable").forEach(function (th) {
        var idx = th.cellIndex;
        th.addEventListener("click", function () {
          if (sortCol === idx) { sortAsc = !sortAsc; } else { sortCol = idx; sortAsc = th.dataset.dir !== "desc"; }
          table.querySelectorAll("th .sort-arrow").forEach(function (a) { a.remove(); });
          var arrow = document.createElement("span");
          arrow.className = "sort-arrow";
          arrow.textContent = sortAsc ? " ▲" : " ▼";
          th.appendChild(arrow);
          var tbody = table.querySelector("tbody");
          var rows = Array.from(tbody.querySelectorAll("tr"));
          rows.sort(function (a, b) {
            var at = a.cells[idx].getAttribute("data-sort") || a.cells[idx].textContent.trim();
            var bt = b.cells[idx].getAttribute("data-sort") || b.cells[idx].textContent.trim();
            var an = parseFloat(at), bn = parseFloat(bt);
            var c = (!isNaN(an) && !isNaN(bn)) ? an - bn : at.localeCompare(bt);
            return sortAsc ? c : -c;
          });
          rows.forEach(function (r) { tbody.appendChild(r); });
        });
      });
    });
    </script>`

const combatTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Wildlife Combat - Spacemolt KB</title>
    <link rel="stylesheet" href="../smui.css">
    <link rel="stylesheet" href="../system.css">
` + wildlifeStyle + `
</head>
<body>
{{.Header}}
    <main class="container page-content">
        <p class="text-muted" style="font-size:0.85em"><a href="index.html">Wildlife</a> &rsaquo; Combat</p>
        <h2>Wildlife Combat</h2>
        <p>How dangerous each species is in a fight, side by side. Click a column heading to sort. <span class="text-muted">Danger and win rate come from every wildlife battle in the public battle feed{{if .StatsMonths}} ({{.StatsMonths}}){{end}}; damage per shot and hit chance come from the battle logs we have exported, so species with few logs have rough numbers. &mdash; means no data yet.</span></p>
        <div class="card" style="padding:0; overflow-x:auto">
        <table class="wl-table sortable">
            <thead>
                <tr>
                    <th class="sortable">Species</th>
                    <th class="sortable">Class</th>
                    <th class="sortable" data-dir="desc" title="Public battle feed: wildlife win rate bucketed into a tier (needs 25+ battles)">Danger</th>
                    <th class="sortable num" data-dir="desc" title="Battles in the public feed">Battles</th>
                    <th class="sortable num" data-dir="desc" title="Share of those battles the wildlife side won">Wildlife wins</th>
                    <th class="sortable">Damage type</th>
                    <th class="sortable num" data-dir="desc" title="Base damage per shot, from exported battle logs">Dmg / shot</th>
                    <th class="sortable num" data-dir="desc" title="Observed hit chance range against its targets">Hit chance</th>
                    <th class="sortable num" data-dir="desc">Hull</th>
                    <th class="sortable num" data-dir="desc">Shield</th>
                    <th class="sortable num" data-dir="desc" title="Kills of this species recorded by our pilots">Kills</th>
                    <th class="sortable num" data-dir="desc" title="Exported battle logs behind the damage and hit columns">Logs</th>
                </tr>
            </thead>
            <tbody>
{{- range .Rows}}
                <tr>
                    <td><a href="{{.ID}}.html">{{.Name}}</a></td>
                    <td><span class="badge {{roleClass .Role}}">{{.Role}}</span></td>
                    <td data-sort="{{.RatingRank}}">{{if .Rating}}<span class="badge {{ratingClass .Rating}}">{{.Rating}}</span>{{else}}<span class="text-muted">{{if .Battles}}unrated{{else}}&mdash;{{end}}</span>{{end}}</td>
                    <td class="num" data-sort="{{.Battles}}">{{if .Battles}}{{.Battles}}{{else}}<span class="text-muted">&mdash;</span>{{end}}</td>
                    <td class="num" data-sort="{{printf "%.2f" .WinPct}}">{{if ge .WinPct 0.0}}{{printf "%.1f" .WinPct}}%{{else}}<span class="text-muted">&mdash;</span>{{end}}</td>
                    <td>{{if .DamageTypes}}{{range $i, $t := .DamageTypes}}{{if $i}} {{end}}<span class="badge">{{$t}}</span>{{end}}{{else}}<span class="text-muted">&mdash;</span>{{end}}</td>
                    <td class="num" data-sort="{{.DmgMax}}">{{if ge .DmgMax 0}}{{intRange .DmgMin .DmgMax}}{{else}}<span class="text-muted">&mdash;</span>{{end}}</td>
                    <td class="num" data-sort="{{printf "%.3f" .HitMax}}">{{if ge .HitMax 0.0}}{{printf "%.2f" .HitMin}}&ndash;{{printf "%.2f" .HitMax}}{{else}}<span class="text-muted">&mdash;</span>{{end}}</td>
                    <td class="num">{{.Hull}}</td>
                    <td class="num">{{.Shield}}</td>
                    <td class="num">{{.Kills}}</td>
                    <td class="num">{{.LogBattles}}</td>
                </tr>
{{- end}}
            </tbody>
        </table>
        </div>
        <p style="margin-top:24px"><a href="index.html">&larr; All wildlife</a></p>
    </main>
` + themeScript + combatSortScript + `
</body>
</html>
`
