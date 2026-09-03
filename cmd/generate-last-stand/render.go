package main

import (
	"encoding/json"
	"fmt"
	"html"
	"sort"
	"strconv"
	"strings"
	"text/template"
)

// opusMagnaID is the catalog id of the titan the whole page is framed
// around.
const opusMagnaID = "opus_magna"

// fixedAssumptions are engine-level assumptions baked into pkg/combatsim's
// swarm model itself (as opposed to Matrix.Assumptions, which documents the
// choices made by this particular generate-last-stand run). The render test
// asserts the exact phrase "no capital weapon bonus" appears somewhere in
// this list.
var fixedAssumptions = []string{
	"attacker swarms get no capital weapon bonus — opus_magna only ever appears as a defender, resolved through the capital-allowed fit path",
	"every ship fires at exactly one target per tick",
	"unlimited ammo, with a 1-tick reload once a magazine runs dry",
	"weapons load their default ammo only",
	"combatants close from range 6 to 0, a reach-gated approach where a weapon fires only once distance is inside its Reach",
	"every combatant fights in fire stance for the whole battle",
	"every hull uses its default_modules fit — no skills, no fitted upgrades",
}

// empireDisplay renders a starterEmpire id as its display name.
var empireDisplay = map[string]string{
	"crimson":  "Crimson",
	"nebula":   "Nebula",
	"outerrim": "Outer-Rim",
	"solarian": "Solarian",
	"voidborn": "Voidborn",
}

// colView is a column's display metadata, pre-escaped for direct use in the
// HTML template (render.go uses text/template, which does not auto-escape).
type colView struct {
	ID         string
	Name       string
	Empire     string // display name, e.g. "Outer-Rim"
	Weapon     string
	DamageType string
}

// capitalize upper-cases s's first rune, leaving the rest unchanged. Used
// only as a display fallback for an empire id outside empireDisplay.
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// displayEmpire renders an empire id (e.g. Column.Empire) as its escaped
// display name — the shared lookup buildColViews and the High-End Setup
// callout (buildHighEndView) both use to label a starter's empire.
func displayEmpire(id string) string {
	if name, ok := empireDisplay[id]; ok {
		return name
	}
	return html.EscapeString(capitalize(id))
}

// buildColViews resolves each matrix column's display metadata.
func buildColViews(cols []Column) []colView {
	out := make([]colView, 0, len(cols))
	for _, c := range cols {
		out = append(out, colView{
			ID:         html.EscapeString(c.ID),
			Name:       html.EscapeString(c.Name),
			Empire:     displayEmpire(c.Empire),
			Weapon:     html.EscapeString(c.Weapon),
			DamageType: html.EscapeString(c.DamageType),
		})
	}
	return out
}

// opusCellView is one empire column's crossover result for the Opus Magna
// callout.
type opusCellView struct {
	ColumnName  string
	Empire      string
	DamageType  string
	Display     string // "∞", "?", or the crossover N
	MedianKills int
	Resolved    bool // false = column didn't resolve at all (see Matrix.Notes)
}

// opusView is the featured Opus Magna callout's render data.
type opusView struct {
	Name  string
	Tier  int
	Class string
	Cells []opusCellView
}

// buildOpusView finds the opus_magna row in m and builds its callout view
// against cols (in column order). Returns nil if the row isn't present in
// this matrix (e.g. a defender subset that excludes it, as in the render
// test) — the page still renders, just without the populated callout.
func buildOpusView(m Matrix, cols []colView) *opusView {
	var row *Row
	for i := range m.Rows {
		if m.Rows[i].ShipID == opusMagnaID {
			row = &m.Rows[i]
			break
		}
	}
	if row == nil {
		return nil
	}
	v := &opusView{
		Name:  html.EscapeString(row.Name),
		Tier:  row.Tier,
		Class: html.EscapeString(row.Class),
	}
	for _, c := range cols {
		cv := opusCellView{ColumnName: c.Name, Empire: c.Empire, DamageType: c.DamageType}
		cell := row.Cells[c.ID]
		switch {
		case cell == nil:
			cv.Display = "?"
			cv.Resolved = false
		case cell.N == 0:
			cv.Display = "∞" // ∞
			cv.MedianKills = cell.MedianKills
			cv.Resolved = true
		default:
			cv.Display = strconv.Itoa(cell.N)
			cv.MedianKills = cell.MedianKills
			cv.Resolved = true
		}
		v.Cells = append(v.Cells, cv)
	}
	return v
}

// opusSpread describes which column the titan falls to fastest/slowest,
// for a data-driven sentence in the damage-type explainer. OK is false when
// there isn't at least one finite, resolved cell to compare.
type opusSpread struct {
	LowName, HighName string
	LowN, HighN       int
	OK                bool
}

func computeOpusSpread(v *opusView) opusSpread {
	var s opusSpread
	if v == nil {
		return s
	}
	for _, c := range v.Cells {
		if !c.Resolved || c.Display == "∞" {
			continue
		}
		n, err := strconv.Atoi(c.Display)
		if err != nil {
			continue
		}
		if !s.OK || n < s.LowN {
			s.LowN, s.LowName = n, c.ColumnName
		}
		if !s.OK || n > s.HighN {
			s.HighN, s.HighName = n, c.ColumnName
		}
		s.OK = true
	}
	return s
}

// crossoverDisplay renders a crossover N as the page's display convention:
// "∞" for 0 (no crossing found within the probe cap), otherwise the number
// itself. Used by the High-End Setup and Multi-Opus Effect callouts, which
// (unlike buildOpusView/buildLowEndView) work from already-resolved N ints
// rather than *CellResult, so there's no separate "missing cell" case here.
func crossoverDisplay(n int) string {
	if n == 0 {
		return "∞"
	}
	return strconv.Itoa(n)
}

// highEndRowView is one empire starter's stock-vs-high-end crossover row in
// the High-End Setup callout.
type highEndRowView struct {
	StarterName string // pre-escaped
	Empire      string // pre-escaped display name
	StockN      string
	HighEndN    string
	Ratio       string // e.g. "1.53×", "" when StockN is ∞ or zero
}

// highEndView is the High-End Setup callout's render data.
type highEndView struct {
	FitName string // pre-escaped
	Rows    []highEndRowView
}

// buildHighEndView renders d's computed crossover figures. Returns nil if d
// is nil or empty (see buildHighEndData) so the page renders without the
// callout rather than showing an empty table.
func buildHighEndView(d *HighEndData) *highEndView {
	if d == nil || len(d.Rows) == 0 {
		return nil
	}
	v := &highEndView{FitName: html.EscapeString(d.FitName)}
	for _, r := range d.Rows {
		rv := highEndRowView{
			StarterName: html.EscapeString(r.StarterName),
			Empire:      displayEmpire(r.Empire),
			StockN:      crossoverDisplay(r.StockN),
			HighEndN:    crossoverDisplay(r.HighEndN),
		}
		if r.StockN > 0 && r.HighEndN > 0 {
			rv.Ratio = fmt.Sprintf("%.2f×", float64(r.HighEndN)/float64(r.StockN))
		}
		v.Rows = append(v.Rows, rv)
	}
	return v
}

// multiOpusRowView is one titan-count D's crossover row in the Multi-Opus
// Effect callout.
type multiOpusRowView struct {
	D        int
	DogpileN string
	DogpileX string // "×N1" ratio, e.g. "1.54×"
	SpreadN  string
	SpreadX  string
}

// multiOpusView is the Multi-Opus Effect callout's render data.
type multiOpusView struct {
	N1   int
	Rows []multiOpusRowView // D=2..multiOpusDMax
}

// buildMultiOpusView renders d's computed crossover figures. Returns nil if
// d is nil (see buildMultiOpusData) so the page renders without the callout
// rather than showing an empty table.
func buildMultiOpusView(d *MultiOpusData) *multiOpusView {
	if d == nil {
		return nil
	}
	v := &multiOpusView{N1: d.N1}
	for _, r := range d.Rows {
		rv := multiOpusRowView{D: r.D, DogpileN: crossoverDisplay(r.DogpileN), SpreadN: crossoverDisplay(r.SpreadN)}
		if r.DogpileN > 0 {
			rv.DogpileX = fmt.Sprintf("%.2f×", float64(r.DogpileN)/float64(d.N1))
		}
		if r.SpreadN > 0 {
			rv.SpreadX = fmt.Sprintf("%.2f×", float64(r.SpreadN)/float64(d.N1))
		}
		v.Rows = append(v.Rows, rv)
	}
	return v
}

// lowEndRowView is one Tier-0 starter hull's crossover results as a
// defender against the five starter attacker columns, for the Tier-0
// mirror table in the low-end callout.
type lowEndRowView struct {
	Name  string   // pre-escaped
	Cells []string // pre-escaped display value ("∞", "?", or the crossover N), in column order
}

// lowEndView is the companion "how easily most hulls fall" callout's
// render data — the mirror of opusView. Where the Opus Magna callout
// reports the single hardest target in the whole matrix, this reports how
// common the opposite outcome is: matchups that resolve to a single
// starter kill, a few of the most fragile hulls in the catalog, and the
// Tier-0 starters' own crossover against each other. Returns via
// buildLowEndView, which is nil only when the matrix has no finite cells
// to report on (e.g. an empty defender set).
type lowEndView struct {
	FiniteCount  int      // finite (n>0) cells across all rows and the given columns
	OneCount     int      // of FiniteCount, how many have n==1
	LeTwoCount   int      // of FiniteCount, how many have n<=2 (includes OneCount)
	FragileHulls []string // pre-escaped names of hulls with n==1 against every column, capped
	Tier0Rows    []lowEndRowView
}

// maxFragileHulls caps how many "falls to one ship on every column" example
// hulls the low-end callout lists — the matrix has dozens (see
// buildLowEndView), but the callout only needs a few illustrative names.
const maxFragileHulls = 6

// buildLowEndView scans m for the opposite extreme of the Opus Magna
// callout: it counts, across every row's cells in cols, how many finite
// crossover results are N=1 (or N<=2), collects a capped, stably-ordered
// (tier then name) list of hulls that fall to N=1 against every column in
// cols, and builds the Tier-0 mirror table — each Tier-0 (starter) row in m
// as a defender, against every column in cols, in column order so it lines
// up with the table's header. All figures are computed from m; nothing is
// hardcoded. Returns nil if m has no finite cells in cols at all.
func buildLowEndView(m Matrix, cols []colView) *lowEndView {
	v := &lowEndView{}

	type fragile struct {
		tier int
		name string
	}
	var fragileHulls []fragile
	for _, r := range m.Rows {
		allOnes := len(cols) > 0
		for _, c := range cols {
			cell := r.Cells[c.ID]
			if cell == nil || cell.N == 0 {
				allOnes = false
				continue
			}
			v.FiniteCount++
			if cell.N <= 2 {
				v.LeTwoCount++
			}
			if cell.N == 1 {
				v.OneCount++
			} else {
				allOnes = false
			}
		}
		if allOnes {
			fragileHulls = append(fragileHulls, fragile{r.Tier, r.Name})
		}
	}
	if v.FiniteCount == 0 {
		return nil
	}

	sort.Slice(fragileHulls, func(i, j int) bool {
		if fragileHulls[i].tier != fragileHulls[j].tier {
			return fragileHulls[i].tier < fragileHulls[j].tier
		}
		return fragileHulls[i].name < fragileHulls[j].name
	})
	for i, f := range fragileHulls {
		if i >= maxFragileHulls {
			break
		}
		v.FragileHulls = append(v.FragileHulls, html.EscapeString(f.name))
	}

	for _, r := range m.Rows {
		if r.Tier != 0 {
			continue
		}
		row := lowEndRowView{Name: html.EscapeString(r.Name)}
		for _, c := range cols {
			cell := r.Cells[c.ID]
			switch {
			case cell == nil:
				row.Cells = append(row.Cells, "?")
			case cell.N == 0:
				row.Cells = append(row.Cells, "∞")
			default:
				row.Cells = append(row.Cells, strconv.Itoa(cell.N))
			}
		}
		v.Tier0Rows = append(v.Tier0Rows, row)
	}
	sort.Slice(v.Tier0Rows, func(i, j int) bool { return v.Tier0Rows[i].Name < v.Tier0Rows[j].Name })

	return v
}

// jsonForScript marshals v to JSON and escapes it for safe embedding inside
// an inline <script> block as a bare object literal: '<', '>', '&' and the
// JS line/paragraph separators never occur outside JSON string values, so a
// global replace keeps the JSON valid while guaranteeing it can't smuggle a
// "</script>" (or similar) sequence out of the tag.
func jsonForScript(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	s := string(raw)
	s = strings.ReplaceAll(s, "<", "\\u003c")
	s = strings.ReplaceAll(s, ">", "\\u003e")
	s = strings.ReplaceAll(s, "&", "\\u0026")
	s = strings.ReplaceAll(s, " ", "\\u2028")
	s = strings.ReplaceAll(s, " ", "\\u2029")
	return s, nil
}

// pageData is the render.go template's top-level data.
type pageData struct {
	Title        string
	GeneratedUTC string
	RowCount     int
	Columns      []colView
	Assumptions  []string // pre-escaped
	Opus         *opusView
	Spread       opusSpread
	HighEnd      *highEndView
	MultiOpus    *multiOpusView
	LowEnd       *lowEndView
	MatrixJSON   string
}

// RenderPage builds the self-contained "Last Stand" KB page for matrix m:
// an Opus Magna callout, the High-End Setup and Multi-Opus Effect callouts
// (from the separately-computed highEnd/multiOpus datasets, either of which
// may be nil), the low-end companion callout, the full sortable/filterable
// crossover table, a damage-type explainer, an assumptions box, and the
// embedded matrix JSON that drives all client-side interactivity.
func RenderPage(m Matrix, highEnd *HighEndData, multiOpus *MultiOpusData) (string, error) {
	cols := buildColViews(m.Columns)
	opus := buildOpusView(m, cols)
	spread := computeOpusSpread(opus)
	highEndV := buildHighEndView(highEnd)
	multiOpusV := buildMultiOpusView(multiOpus)
	lowEnd := buildLowEndView(m, cols)

	assumptions := make([]string, 0, len(fixedAssumptions)+len(m.Assumptions))
	for _, a := range fixedAssumptions {
		assumptions = append(assumptions, html.EscapeString(a))
	}
	for _, a := range m.Assumptions {
		assumptions = append(assumptions, html.EscapeString(a))
	}

	matrixJSON, err := jsonForScript(m)
	if err != nil {
		return "", fmt.Errorf("marshal matrix for embed: %w", err)
	}

	data := pageData{
		Title:        "Last Stand: Swarm vs Titan",
		GeneratedUTC: html.EscapeString(m.GeneratedUTC),
		RowCount:     len(m.Rows),
		Columns:      cols,
		Assumptions:  assumptions,
		Opus:         opus,
		Spread:       spread,
		HighEnd:      highEndV,
		MultiOpus:    multiOpusV,
		LowEnd:       lowEnd,
		MatrixJSON:   matrixJSON,
	}

	tmpl, err := template.New("last_stand").Parse(pageTemplate)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}
	return buf.String(), nil
}

const pageTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}} - Did You Know? - Spacemolt KB</title>
    <link rel="stylesheet" href="../smui.css">
    <style>
        .ls-lede { max-width: 72ch; }
        .ls-callout {
            background: hsl(var(--card)); color: hsl(var(--card-foreground));
            border: 1px solid hsl(var(--border)); border-left: 4px solid #6c5ce7;
            border-radius: 8px; padding: 20px; margin: 20px 0;
        }
        .ls-callout h3 { margin: 0 0 4px 0; }
        .ls-callout .ls-sub { color: hsl(var(--muted-foreground)); font-size: 0.9em; margin-bottom: 14px; }
        .ls-grid {
            display: grid; grid-template-columns: repeat(auto-fit, minmax(160px, 1fr)); gap: 12px;
        }
        .ls-stat {
            background: hsl(var(--accent)); border: 1px solid hsl(var(--border)); border-radius: 6px;
            padding: 10px 12px;
        }
        .ls-stat .label { display: block; }
        .ls-stat .n { font-size: var(--text-stat); font-weight: 500; display: block; margin: 2px 0; }
        .ls-stat .kills { color: hsl(var(--muted-foreground)); font-size: 0.85em; }
        .ls-section { margin: 28px 0; }
        .ls-filters {
            display: flex; flex-wrap: wrap; align-items: flex-end; gap: 12px;
            background: hsl(var(--card)); border: 1px solid hsl(var(--border)); border-radius: 8px;
            padding: 14px; margin: 16px 0;
        }
        .ls-field { display: flex; flex-direction: column; gap: 4px; }
        .ls-field label { font-size: 0.75em; text-transform: uppercase; color: hsl(var(--muted-foreground)); }
        .ls-field input, .ls-field select {
            background: hsl(var(--secondary)); color: hsl(var(--foreground));
            border: 1px solid hsl(var(--border)); border-radius: 6px; padding: 6px 8px; font-size: 0.9em;
        }
        .ls-field input[type="number"] { width: 7em; }
        #f-search { min-width: 200px; }
        .ls-table-wrap { overflow-x: auto; border: 1px solid hsl(var(--border)); border-radius: 8px; }
        #matrix { min-width: 720px; }
        #matrix th[data-key] { cursor: pointer; user-select: none; white-space: nowrap; }
        #matrix th[data-key]:hover { color: hsl(var(--foreground)); }
        #matrix td.ls-cell { cursor: pointer; text-align: right; font-variant-numeric: tabular-nums; }
        #matrix td.ls-cell:hover { text-decoration: underline; }
        #matrix td.ls-inf { color: hsl(var(--muted-foreground)); }
        #matrix-empty { padding: 16px; color: hsl(var(--muted-foreground)); }
        .ls-drawer {
            position: fixed; top: 0; right: 0; height: 100%; width: min(420px, 100%);
            background: hsl(var(--card)); color: hsl(var(--card-foreground));
            border-left: 1px solid hsl(var(--border)); box-shadow: -8px 0 24px rgba(0,0,0,0.25);
            padding: 18px; overflow-y: auto; transform: translateX(100%);
            transition: transform 0.2s ease; z-index: 50;
        }
        .ls-drawer.open { transform: translateX(0); }
        .ls-drawer h4 { margin: 0 0 4px 0; padding-right: 24px; }
        .ls-drawer .ls-drawer-sub { color: hsl(var(--muted-foreground)); font-size: 0.85em; margin-bottom: 12px; }
        .ls-drawer-close {
            position: absolute; top: 14px; right: 14px; background: none; border: 0;
            color: hsl(var(--muted-foreground)); font-size: 1.2em; cursor: pointer; line-height: 1;
        }
        .ls-drawer-close:hover { color: hsl(var(--foreground)); }
        .ls-drawer canvas { width: 100%; height: 220px; background: hsl(var(--secondary)); border-radius: 6px; }
        .ls-drawer .ls-drawer-note { font-size: 0.85em; color: hsl(var(--muted-foreground)); margin-top: 10px; }
        .ls-backdrop {
            position: fixed; inset: 0; background: rgba(0,0,0,0.35); z-index: 40;
            opacity: 0; pointer-events: none; transition: opacity 0.2s ease;
        }
        .ls-backdrop.open { opacity: 1; pointer-events: auto; }
        .ls-dmg-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 14px; margin-top: 12px; }
        .ls-dmg-card { border: 1px solid hsl(var(--border)); border-radius: 6px; padding: 10px 12px; }
        .ls-dmg-card h4 { margin: 0 0 6px 0; font-size: 1em; }
        .ls-dmg-card p { margin: 0; font-size: 0.88em; color: hsl(var(--muted-foreground)); }
        .ls-assumptions ul { margin: 8px 0 0 1.2em; padding: 0; }
        .ls-assumptions li { margin: 4px 0; font-size: 0.92em; }
    </style>
</head>
<body>
    <header class="site-header">
        <h1><a href="../" style="color:inherit;text-decoration:none">Spacemolt KB</a></h1>
        <nav>
            <a href="../">Home</a>
            <a href="../systems/index.html">Systems</a>
            <a href="../items/index.html">Items</a>
            <a href="../recipes/index.html">Recipes</a>
            <a href="../skills/index.html">Skills</a>
            <a href="../ships/index.html">Ships</a>
            <a href="../facilities/index.html">Facilities</a>
            <a href="../resources/index.html">Resources</a>
            <a href="../missions/index.html">Missions</a>
            <a href="./">Did You Know?</a>
            <button class="theme-toggle" id="theme-toggle" aria-label="Toggle theme">
                <svg class="icon-sun" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>
                <svg class="icon-moon" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>
            </button>
        </nav>
    </header>
    <main class="container page-content">
        <div class="breadcrumb"><a href="./">Did You Know?</a> / Last Stand: Swarm vs Titan</div>
        <h2>Last Stand: Swarm vs Titan</h2>
        <p class="text-muted mt-1 ls-lede">
            Picture facing a swarm of 1,000 fifth-graders. One of them can't lay a glove on you — but pile on
            enough, and even the toughest opponent goes down. This page runs that experiment for every hull in
            the catalog: starting from each empire's stock starter ship, how many identical attackers does it
            take to beat a given defender more often than not? The answer is the <strong>crossover N</strong> —
            the smallest attacking swarm whose measured win rate crosses 50%, found by live-simulating
            {{.RowCount}} defenders against 5 empire-starter swarms with <a href="#ls-assumptions">the assumptions below</a>.
        </p>

        {{if .Opus}}
        <div class="ls-callout">
            <h3>Featured: {{.Opus.Name}}</h3>
            <div class="ls-sub">Tier {{.Opus.Tier}} · {{.Opus.Class}} — the galaxy's biggest hull. How big a swarm does it take?</div>
            <div class="ls-grid">
                {{range .Opus.Cells}}
                <div class="ls-stat">
                    <span class="label">{{.Empire}}</span>
                    <span class="n">{{.Display}}</span>
                    {{if .Resolved}}<span class="kills">the titan kills {{.MedianKills}} of you first</span>{{else}}<span class="kills">not resolved</span>{{end}}
                </div>
                {{end}}
            </div>
        </div>
        {{else}}
        <div class="ls-callout">
            <h3>Featured: Opus Magna</h3>
            <div class="ls-sub">The galaxy's biggest hull isn't in this particular matrix run.</div>
        </div>
        {{end}}

        {{if .HighEnd}}
        <div class="ls-callout" id="ls-high-end">
            <h3>High-End Setup: same hull, tricked out</h3>
            <div class="ls-sub">
                {{.HighEnd.FitName}} — a real Combat Drone gunline Opus Magna, reconstructed from a played battle.
                Its only combat difference from stock: flat damage reduction pushed from 35% up to the 75% cap.
            </div>
            <div class="ls-table-wrap">
                <table>
                    <thead>
                        <tr><th>Starter</th><th>Stock N</th><th>High-end N</th><th>&times;more</th></tr>
                    </thead>
                    <tbody>
                        {{range .HighEnd.Rows}}
                        <tr>
                            <td>{{.StarterName}}<br><span class="text-muted">{{.Empire}}</span></td>
                            <td class="ls-cell">{{.StockN}}</td>
                            <td class="ls-cell">{{.HighEndN}}</td>
                            <td class="ls-cell">{{if .Ratio}}{{.Ratio}}{{else}}&mdash;{{end}}</td>
                        </tr>
                        {{end}}
                    </tbody>
                </table>
            </div>
            <p class="text-muted mt-1">
                Roughly 2.6&times; the effective HP (the damage-reduction cap over stock) buys roughly
                1.3&ndash;2&times; the swarm needed to crack it — win rate scales with the swarm's square root of
                effective HP, not linearly.
            </p>
        </div>
        {{end}}

        {{if .MultiOpus}}
        <div class="ls-callout" id="ls-multi-opus">
            <h3>Multi-Opus Effect: facing more than one titan</h3>
            <div class="ls-sub">
                A Prospect swarm needs {{.MultiOpus.N1}} ships to beat a single stock Opus Magna. Add more
                titans, and the threshold depends on a choice the defenders make: concentrate fire, or spread it.
            </div>
            <div class="ls-table-wrap">
                <table>
                    <thead>
                        <tr><th>Titans (D)</th><th>Dogpile N</th><th>&times;N1</th><th>Spread N</th><th>&times;N1</th></tr>
                    </thead>
                    <tbody>
                        <tr>
                            <td>1</td>
                            <td class="ls-cell">{{.MultiOpus.N1}}</td><td class="ls-cell">1.00&times;</td>
                            <td class="ls-cell">{{.MultiOpus.N1}}</td><td class="ls-cell">1.00&times;</td>
                        </tr>
                        {{range .MultiOpus.Rows}}
                        <tr>
                            <td>{{.D}}</td>
                            <td class="ls-cell">{{.DogpileN}}</td><td class="ls-cell">{{.DogpileX}}</td>
                            <td class="ls-cell">{{.SpreadN}}</td><td class="ls-cell">{{.SpreadX}}</td>
                        </tr>
                        {{end}}
                    </tbody>
                </table>
            </div>
            <p class="text-muted mt-1">
                Dogpile (concentrate fire) guarantees kills fastest; spread (parallelize) avoids wasting a
                capital's overkill firepower on an already-dying target — a player choice, not a fixed rule. The
                measured threshold sits between two theoretical bounds: N1&times;&radic;D (pure firepower scaling)
                and the Lanchester square-law bound N1&times;&radic;(D(D+1)/2).
            </p>
        </div>
        {{end}}

        {{if .LowEnd}}
        <div class="ls-callout" id="ls-low-end">
            <h3>The other end of the spectrum: how easily most hulls fall</h3>
            <div class="ls-sub">
                One Opus Magna eats a swarm of dozens of starters before it goes down — but flip the question
                around, and a single starter is often all it takes.
            </div>
            <p class="text-muted">
                <strong>{{.LowEnd.OneCount}}</strong> of the <strong>{{.LowEnd.FiniteCount}}</strong> rated matchups
                on this page fall to a single starter (N=1){{if .LowEnd.LeTwoCount}} &mdash;
                <strong>{{.LowEnd.LeTwoCount}}</strong> fall to one or two{{end}}.
            </p>
            {{if .LowEnd.FragileHulls}}
            <p class="text-muted">
                Most fragile: every one of the five starter swarms needs just a single ship to beat
                {{range $i, $n := .LowEnd.FragileHulls}}{{if $i}}, {{end}}<strong>{{$n}}</strong>{{end}}.
            </p>
            {{end}}
            {{if .LowEnd.Tier0Rows}}
            <p class="text-muted mt-1">
                The mirror match: how many ships a lone starter swarm defeats before it dies itself, against each
                of the five Tier-0 starter hulls as a defender.
            </p>
            <div class="ls-table-wrap">
                <table>
                    <thead>
                        <tr>
                            <th>Starter (defender)</th>
                            {{range .Columns}}<th title="{{.Weapon}} ({{.DamageType}})">{{.Name}}<br><span class="text-muted">{{.Empire}}</span></th>{{end}}
                        </tr>
                    </thead>
                    <tbody>
                        {{range .LowEnd.Tier0Rows}}
                        <tr>
                            <td>{{.Name}}</td>
                            {{range .Cells}}<td class="ls-cell">{{.}}</td>{{end}}
                        </tr>
                        {{end}}
                    </tbody>
                </table>
            </div>
            {{end}}
        </div>
        {{end}}

        <div class="ls-section">
            <h3>Damage types: why the swarm's weapon matters</h3>
            <p class="text-muted">
                Every hull's fit fires one damage type in this model. Shields and armor don't treat all three
                the same way — this table (measured from the combat engine, not a guess) is why a Crimson swarm
                and a Voidborn swarm need very different headcounts to crack the same target.
            </p>
            <div class="ls-dmg-grid">
                <div class="ls-dmg-card">
                    <h4>Kinetic</h4>
                    <p>Full damage to shields (100%), but armor is 1.5&times; as effective against it once it
                        reaches hull — armor is kinetic's hard counter.</p>
                </div>
                <div class="ls-dmg-card">
                    <h4>Energy</h4>
                    <p>Only 75% to shields <em>and</em> armor is 25% less effective against it than baseline —
                        even, unspectacular damage against either layer.</p>
                </div>
                <div class="ls-dmg-card">
                    <h4>EM</h4>
                    <p>Full damage to shields, and armor treats it at baseline effectiveness — a middle ground
                        between kinetic and energy on the hull.</p>
                </div>
            </div>
            {{if .Spread.OK}}
            <p class="text-muted mt-2">
                In this matrix, the titan falls fastest to a <strong>{{.Spread.LowName}}</strong> swarm
                (N={{.Spread.LowN}}) and holds out longest against <strong>{{.Spread.HighName}}</strong>
                (N={{.Spread.HighN}}) — a direct read of how its shield/armor mix answers each damage type above,
                not a hand-wave.
            </p>
            {{end}}
        </div>

        <div class="ls-section ls-assumptions" id="ls-assumptions">
            <h3>Assumptions</h3>
            <p class="text-muted">Every crossover N on this page comes from the same fixed rule set:</p>
            <ul>
                {{range .Assumptions}}<li>{{.}}</li>{{end}}
            </ul>
            <p class="text-muted mt-1">Generated {{.GeneratedUTC}} · {{.RowCount}} defender hulls.</p>
        </div>

        <div class="ls-section">
            <h3>The full matrix</h3>
            <p class="text-muted">Click any column header to sort, or a number to see the crossover curve. Numbers are the swarm size N needed for a &gt;50% win rate; &infin; means no swarm size tried (up to the probe cap) got there.</p>

            <div class="ls-filters">
                <div class="ls-field">
                    <label for="f-search">Search</label>
                    <input type="text" id="f-search" placeholder="Ship name…">
                </div>
                <div class="ls-field">
                    <label for="f-tier">Tier</label>
                    <select id="f-tier"><option value="">Any</option></select>
                </div>
                <div class="ls-field">
                    <label for="f-class">Class</label>
                    <select id="f-class"><option value="">Any</option></select>
                </div>
                <div class="ls-field">
                    <label for="f-col">Empire column</label>
                    <select id="f-col"><option value="">Any</option>{{range .Columns}}<option value="{{.ID}}">{{.Name}} ({{.Empire}})</option>{{end}}</select>
                </div>
                <div class="ls-field">
                    <label for="f-min">Min N</label>
                    <input type="number" id="f-min" min="0">
                </div>
                <div class="ls-field">
                    <label for="f-max">Max N</label>
                    <input type="number" id="f-max" min="0">
                </div>
            </div>

            <div class="ls-table-wrap">
                <table id="matrix">
                    <thead>
                        <tr>
                            <th data-key="name">Ship</th>
                            <th data-key="tier">Tier</th>
                            <th data-key="class">Class</th>
                            {{range .Columns}}<th data-key="col:{{.ID}}" title="{{.Weapon}} ({{.DamageType}})">{{.Name}}<br><span class="text-muted">{{.Empire}}</span></th>{{end}}
                        </tr>
                    </thead>
                    <tbody id="matrix-body"></tbody>
                </table>
                <div id="matrix-empty" hidden>No ships match these filters.</div>
            </div>
        </div>
    </main>

    <div class="ls-backdrop" id="ls-backdrop"></div>
    <aside class="ls-drawer" id="ls-drawer" aria-hidden="true">
        <button type="button" class="ls-drawer-close" id="ls-drawer-close" aria-label="Close">&times;</button>
        <h4 id="ls-drawer-title"></h4>
        <div class="ls-drawer-sub" id="ls-drawer-sub"></div>
        <canvas id="ls-drawer-canvas" width="360" height="220"></canvas>
        <div class="ls-drawer-note" id="ls-drawer-note"></div>
    </aside>

    <script>const MATRIX_DATA = {{.MatrixJSON}};</script>
    <script>
    (function () {
        var toggle = document.getElementById('theme-toggle');
        var root = document.documentElement;
        if (localStorage.getItem('theme') === 'dark') root.classList.add('dark');
        toggle.addEventListener('click', function () {
            root.classList.toggle('dark');
            localStorage.setItem('theme', root.classList.contains('dark') ? 'dark' : 'light');
        });
    })();

    (function () {
        var data = MATRIX_DATA;
        var rows = data.rows || [];
        var cols = data.columns || [];
        var colsById = {};
        cols.forEach(function (c) { colsById[c.id] = c; });

        // --- helpers -------------------------------------------------------
        function cellOf(row, colID) { return row.cells ? row.cells[colID] : undefined; }
        // Both a missing cell (attacker column never resolved) and a measured
        // N==0 (no crossover within the probe cap) are "infinite" for sorting
        // and range filtering — see the Task 7 interface note this page was
        // built against.
        function cellN(row, colID) {
            var c = cellOf(row, colID);
            if (!c || c.n === 0) return Infinity;
            return c.n;
        }
        function cellDisplay(row, colID) {
            var c = cellOf(row, colID);
            if (!c) return '?';
            if (c.n === 0) return '∞';
            return String(c.n);
        }

        // --- filter controls -------------------------------------------------
        var searchEl = document.getElementById('f-search');
        var tierEl = document.getElementById('f-tier');
        var classEl = document.getElementById('f-class');
        var colEl = document.getElementById('f-col');
        var minEl = document.getElementById('f-min');
        var maxEl = document.getElementById('f-max');

        function unique(vals) {
            var seen = {}, out = [];
            vals.forEach(function (v) { if (!(v in seen)) { seen[v] = true; out.push(v); } });
            return out;
        }
        unique(rows.map(function (r) { return r.tier; })).sort(function (a, b) { return a - b; }).forEach(function (t) {
            var o = document.createElement('option'); o.value = String(t); o.textContent = 'Tier ' + t; tierEl.appendChild(o);
        });
        unique(rows.map(function (r) { return r.class; })).sort().forEach(function (c) {
            var o = document.createElement('option'); o.value = c; o.textContent = c; classEl.appendChild(o);
        });

        // --- sort state --------------------------------------------------
        var sortKey = 'name', sortDir = 1;
        document.querySelectorAll('#matrix th[data-key]').forEach(function (th) {
            th.addEventListener('click', function () {
                var key = th.getAttribute('data-key');
                if (sortKey === key) { sortDir = -sortDir; } else { sortKey = key; sortDir = 1; }
                render();
            });
        });

        function compareRows(a, b) {
            var va, vb;
            if (sortKey === 'name' || sortKey === 'class') {
                va = a[sortKey]; vb = b[sortKey];
                return sortDir * va.localeCompare(vb);
            }
            if (sortKey === 'tier') {
                return sortDir * (a.tier - b.tier);
            }
            var colID = sortKey.slice(4);
            return sortDir * (cellN(a, colID) - cellN(b, colID));
        }

        // --- render --------------------------------------------------------
        var tbody = document.getElementById('matrix-body');
        var emptyEl = document.getElementById('matrix-empty');

        function render() {
            var q = searchEl.value.trim().toLowerCase();
            var tier = tierEl.value, cls = classEl.value, col = colEl.value;
            var min = minEl.value === '' ? -Infinity : parseFloat(minEl.value);
            var max = maxEl.value === '' ? Infinity : parseFloat(maxEl.value);

            var filtered = rows.filter(function (r) {
                if (q && r.name.toLowerCase().indexOf(q) === -1) return false;
                if (tier !== '' && String(r.tier) !== tier) return false;
                if (cls !== '' && r.class !== cls) return false;
                if (col !== '') {
                    var n = cellN(r, col);
                    if (n < min || n > max) return false;
                }
                return true;
            });
            filtered.sort(compareRows);

            tbody.textContent = '';
            filtered.forEach(function (r) {
                var tr = document.createElement('tr');
                var tdName = document.createElement('td'); tdName.textContent = r.name; tr.appendChild(tdName);
                var tdTier = document.createElement('td'); tdTier.textContent = String(r.tier); tr.appendChild(tdTier);
                var tdClass = document.createElement('td'); tdClass.textContent = r.class; tr.appendChild(tdClass);
                cols.forEach(function (c) {
                    var td = document.createElement('td');
                    td.className = 'ls-cell';
                    var disp = cellDisplay(r, c.id);
                    td.textContent = disp;
                    if (disp === '∞' || disp === '?') td.classList.add('ls-inf');
                    td.addEventListener('click', function () { openDrawer(r, c.id); });
                    tr.appendChild(td);
                });
                tbody.appendChild(tr);
            });
            emptyEl.hidden = filtered.length > 0;
        }
        [searchEl, tierEl, classEl, colEl, minEl, maxEl].forEach(function (el) {
            el.addEventListener('input', render);
            el.addEventListener('change', render);
        });
        render();

        // --- drawer ----------------------------------------------------
        var drawer = document.getElementById('ls-drawer');
        var backdrop = document.getElementById('ls-backdrop');
        var drawerTitle = document.getElementById('ls-drawer-title');
        var drawerSub = document.getElementById('ls-drawer-sub');
        var drawerNote = document.getElementById('ls-drawer-note');
        var canvas = document.getElementById('ls-drawer-canvas');

        function closeDrawer() {
            drawer.classList.remove('open'); drawer.setAttribute('aria-hidden', 'true');
            backdrop.classList.remove('open');
        }
        document.getElementById('ls-drawer-close').addEventListener('click', closeDrawer);
        backdrop.addEventListener('click', closeDrawer);

        function openDrawer(row, colID) {
            var col = colsById[colID];
            var cell = cellOf(row, colID);
            drawerTitle.textContent = row.name + ' vs ' + (col ? col.name : colID);
            drawerSub.textContent = col ? (col.weapon + ' · ' + col.damage_type + ' damage') : '';

            var ctx = canvas.getContext('2d');
            ctx.clearRect(0, 0, canvas.width, canvas.height);
            if (!cell || !cell.curve || cell.curve.length === 0) {
                drawerNote.textContent = 'This attacker column did not resolve — no simulation data.';
            } else {
                drawCurve(ctx, cell.curve, cell.n);
                var nText = cell.n === 0 ? '∞ (no crossover found)' : String(cell.n);
                drawerNote.textContent = 'Crossover N = ' + nText + '. Median attackers the defender kills at that swarm size: ' + cell.median_kills + '.';
            }
            drawer.classList.add('open'); drawer.setAttribute('aria-hidden', 'false');
            backdrop.classList.add('open');
        }

        // Plots win rate (y) against swarm size N (x, log scale) from a
        // Crossover run's probed points, with a 50% threshold line and the
        // crossover point (if any) marked.
        function drawCurve(ctx, curve, crossoverN) {
            var w = canvas.width, h = canvas.height, pad = 30;
            var ns = curve.map(function (p) { return Math.max(p.n, 1); });
            var logMin = Math.log10(Math.min.apply(null, ns));
            var logMax = Math.log10(Math.max.apply(null, ns));
            if (logMax === logMin) logMax = logMin + 1;
            function x(n) { return pad + (Math.log10(Math.max(n, 1)) - logMin) / (logMax - logMin) * (w - pad * 2); }
            function y(p) { return h - pad - p * (h - pad * 2); }

            var muted = getComputedStyle(document.body).getPropertyValue('color') || '#888';
            ctx.strokeStyle = 'rgba(128,128,128,0.4)';
            ctx.lineWidth = 1;
            ctx.beginPath(); ctx.moveTo(pad, y(0.5)); ctx.lineTo(w - pad, y(0.5)); ctx.stroke();
            ctx.setLineDash([]);
            ctx.beginPath(); ctx.moveTo(pad, pad); ctx.lineTo(pad, h - pad); ctx.lineTo(w - pad, h - pad); ctx.stroke();

            ctx.fillStyle = 'rgba(128,128,128,0.8)';
            ctx.font = '10px monospace';
            ctx.fillText('N=' + Math.round(Math.pow(10, logMin)), pad, h - pad + 12);
            ctx.textAlign = 'right';
            ctx.fillText('N=' + Math.round(Math.pow(10, logMax)), w - pad, h - pad + 12);
            ctx.textAlign = 'left';
            ctx.fillText('50%', pad + 2, y(0.5) - 3);

            var sorted = curve.slice().sort(function (a, b) { return a.n - b.n; });
            ctx.strokeStyle = '#6c5ce7'; ctx.lineWidth = 2;
            ctx.beginPath();
            sorted.forEach(function (p, i) {
                var px = x(p.n), py = y(p.p_win);
                if (i === 0) ctx.moveTo(px, py); else ctx.lineTo(px, py);
            });
            ctx.stroke();
            ctx.fillStyle = '#6c5ce7';
            sorted.forEach(function (p) {
                ctx.beginPath(); ctx.arc(x(p.n), y(p.p_win), 2.5, 0, Math.PI * 2); ctx.fill();
            });
            if (crossoverN && crossoverN > 0) {
                var cp = sorted.filter(function (p) { return p.n === crossoverN; })[0];
                if (cp) {
                    ctx.fillStyle = '#ff7a4d';
                    ctx.beginPath(); ctx.arc(x(cp.n), y(cp.p_win), 5, 0, Math.PI * 2); ctx.fill();
                }
            }
        }
    })();
    </script>
</body>
</html>
`
