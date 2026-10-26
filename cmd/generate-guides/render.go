package main

import (
	"bytes"
	htmltpl "html/template"
	"slices"
	"strconv"
	"strings"

	"github.com/rsned/spacemolt-kb/internal/kbnav"
	"github.com/rsned/spacemolt-kb/pkg/catalog"
	"github.com/rsned/spacemolt-kb/pkg/stationplan"
	"github.com/yuin/goldmark"
)

// markdown renders with goldmark's safe defaults: raw HTML in a guide is
// dropped, so the only markup on the page is what the generator emits.
var markdown = goldmark.New()

// renderer turns parsed guides into page view models. warnings collects
// unresolved [[id]] references.
type renderer struct {
	facilities map[string]catalog.Facility
	links      *stationplan.Linker
	warnings   []string
}

type linkView struct {
	Name, Href string // Href empty → plain text
}

type qtyView struct {
	Item     linkView
	Qty      string
	PerCycle string
	PerDay   string
}

type altView struct {
	Facility linkView
	Credits  string
	PerDay   string // total upkeep units per day
	Gives    string
}

type measuredView struct {
	Date, Source string
	Upkeep       []qtyView
}

type gridView struct {
	Power, LifeSupport               string // "supply / draw"
	PowerDeficit, LifeSupportDeficit bool
}

type stepView struct {
	Num        int
	Title      string
	Facility   *linkView
	Gives      string
	Credits    string
	Materials  []qtyView
	BuildTime  int
	Upkeep     []qtyView
	NoUpkeep   bool
	Facts      []fact
	Notes      htmltpl.HTML
	Measured   *measuredView
	Alts       []altView
	SoFarItems int
	SoFarDay   string
	Grid       gridView
}

type sidebarView struct {
	Upkeep    []qtyView
	Credits   string
	Materials []qtyView
	Grid      gridView
}

type pageView struct {
	Header  htmltpl.HTML
	Title   string
	Intro   htmltpl.HTML
	Steps   []stepView
	Sidebar sidebarView
	Body    htmltpl.HTML
}

// md expands [[id]] refs, records unresolved ones against the guide, and
// renders the markdown.
func (r *renderer) md(g *guide, src string) (htmltpl.HTML, error) {
	expanded, unresolved := r.links.ExpandRefs(src)
	for _, id := range unresolved {
		r.warnings = append(r.warnings, g.file+": unresolved reference [["+id+"]]")
	}
	var buf bytes.Buffer
	if err := markdown.Convert([]byte(expanded), &buf); err != nil {
		return "", err
	}
	return htmltpl.HTML(buf.String()), nil //nolint:gosec // goldmark safe mode strips raw HTML
}

func (r *renderer) link(id string) linkView {
	href, _ := r.links.Href(id)
	return linkView{Name: r.links.Name(id), Href: href}
}

// qtys renders an item→quantity map sorted by display name.
func (r *renderer) qtys(m map[string]int, perCycle bool) []qtyView {
	out := make([]qtyView, 0, len(m))
	for id, q := range m {
		v := qtyView{Item: r.link(id)}
		if perCycle {
			v.PerCycle, v.PerDay = commas(q), commas(stationplan.PerDay(q))
		} else {
			v.Qty = commas(q)
		}
		out = append(out, v)
	}
	slices.SortFunc(out, func(a, b qtyView) int { return strings.Compare(a.Item.Name, b.Item.Name) })
	return out
}

func materialMap(ms []catalog.Material) map[string]int {
	out := map[string]int{}
	for _, m := range ms {
		out[m.ItemID] += int(m.Quantity + 0.5)
	}
	return out
}

func upkeepMap(f catalog.Facility) map[string]int {
	out := materialMap(f.MaintenanceInputs)
	if f.MaintenanceFuel > 0 {
		out[stationplan.FuelItemID] += f.MaintenanceFuel
	}
	return out
}

func gives(f catalog.Facility) string {
	var parts []string
	if f.ServiceType != "" {
		parts = append(parts, "service: "+f.ServiceType)
	}
	if f.PowerSupply > 0 {
		parts = append(parts, "power +"+strconv.Itoa(f.PowerSupply))
	}
	if f.LifeSupportSupply > 0 {
		parts = append(parts, "life support +"+strconv.Itoa(f.LifeSupportSupply))
	}
	if f.PowerDraw > 0 {
		parts = append(parts, "draws power "+strconv.Itoa(f.PowerDraw))
	}
	if f.LifeSupportDraw > 0 {
		parts = append(parts, "draws life support "+strconv.Itoa(f.LifeSupportDraw))
	}
	return strings.Join(parts, " · ")
}

func grid(t stationplan.Totals) gridView {
	return gridView{
		Power:              strconv.Itoa(t.PowerSupply) + " / " + strconv.Itoa(t.PowerDraw),
		LifeSupport:        strconv.Itoa(t.LifeSupportSupply) + " / " + strconv.Itoa(t.LifeSupportDraw),
		PowerDeficit:       t.PowerDeficit(),
		LifeSupportDeficit: t.LifeSupportDeficit(),
	}
}

func perDayTotal(m map[string]int) int {
	n := 0
	for _, q := range m {
		n += stationplan.PerDay(q)
	}
	return n
}

// page builds the view for g given its cumulative totals (one per step).
func (r *renderer) page(g *guide, totals []stationplan.Totals) (pageView, error) {
	pv := pageView{Header: htmltpl.HTML(kbnav.Header("../")), Title: g.Title} //nolint:gosec // site header, generated internally
	var err error
	if pv.Intro, err = r.md(g, g.Intro); err != nil {
		return pv, err
	}
	if pv.Body, err = r.md(g, g.body); err != nil {
		return pv, err
	}
	for i, s := range g.Steps {
		t := totals[i]
		sv := stepView{Num: i, Title: s.Title, Facts: s.Facts, Grid: grid(t),
			SoFarItems: len(t.UpkeepPerCycle), SoFarDay: commas(perDayTotal(t.UpkeepPerCycle))}
		if sv.Notes, err = r.md(g, s.Notes); err != nil {
			return pv, err
		}
		if s.Facility != "" {
			f := r.facilities[s.Facility]
			lv := r.link(s.Facility)
			sv.Facility = &lv
			sv.Gives = gives(f)
			sv.Credits = commas(f.BuildCost)
			sv.Materials = r.qtys(materialMap(f.BuildMaterials), false)
			sv.BuildTime = f.BuildTime
			up := upkeepMap(f)
			sv.Upkeep = r.qtys(up, true)
			sv.NoUpkeep = len(up) == 0
		}
		if s.Measured != nil {
			sv.Measured = &measuredView{Date: s.Measured.Date, Source: s.Measured.Source,
				Upkeep: r.qtys(s.Measured.UpkeepPerCycle, true)}
		}
		for _, id := range s.Alternatives {
			f := r.facilities[id]
			sv.Alts = append(sv.Alts, altView{Facility: r.link(id), Credits: commas(f.BuildCost),
				PerDay: commas(perDayTotal(upkeepMap(f))), Gives: gives(f)})
		}
		pv.Steps = append(pv.Steps, sv)
	}
	if n := len(totals); n > 0 {
		last := totals[n-1]
		pv.Sidebar = sidebarView{Upkeep: r.qtys(last.UpkeepPerCycle, true), Credits: commas(last.Credits),
			Materials: r.qtys(last.Materials, false), Grid: grid(last)}
	}
	return pv, nil
}

// commas formats n with thousands separators.
func commas(n int) string {
	s := strconv.Itoa(n)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

const pageStyle = `<style>
        .guide-layout { display: grid; grid-template-columns: minmax(0, 1fr) 300px; gap: 24px; align-items: start; }
        .guide-sidebar { position: sticky; top: 16px; border: 1px solid var(--border); border-radius: 8px; padding: 12px 16px; background: var(--card); font-size: 0.9em; }
        .guide-sidebar h3 { margin: 0 0 4px; font-size: 1em; }
        .guide-sidebar table { width: 100%; }
        .step-card { border: 1px solid var(--border); border-radius: 8px; padding: 12px 16px; margin: 14px 0; background: var(--card); }
        .step-card h3 { margin: 0 0 4px; }
        .step-gives { color: var(--muted-foreground); font-size: 0.9em; margin: 0 0 8px; }
        .step-card table { font-size: 0.9em; margin: 4px 0 8px; }
        .step-card td, .step-card th, .guide-sidebar td { padding: 2px 10px 2px 0; text-align: left; }
        .num { text-align: right !important; font-variant-numeric: tabular-nums; }
        .upkeep { border-left: 3px solid var(--smui-yellow); padding-left: 8px; }
        .no-upkeep { color: var(--muted-foreground); font-style: italic; }
        .measured { border-left: 3px solid var(--smui-green); padding-left: 8px; font-size: 0.9em; }
        .alts { font-size: 0.85em; color: var(--muted-foreground); }
        .so-far { border-top: 1px solid var(--border); margin-top: 8px; padding-top: 6px; font-size: 0.85em; color: var(--muted-foreground); }
        .deficit { color: var(--smui-red); font-weight: 600; }
        .facts td:last-child { color: var(--muted-foreground); font-size: 0.85em; }
        @media (max-width: 900px) {
            .guide-layout { grid-template-columns: 1fr; }
            .guide-sidebar { position: static; order: -1; }
        }
</style>`

const themeScript = `    <script>
    (function() {
        var toggle = document.getElementById('theme-toggle');
        var root = document.documentElement;
        if (localStorage.getItem('theme') === 'dark') root.classList.add('dark');
        if (toggle) toggle.addEventListener('click', function() {
            root.classList.toggle('dark');
            localStorage.setItem('theme', root.classList.contains('dark') ? 'dark' : 'light');
        });
    })();
    </script>`

const pageHead = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}} - Spacemolt KB</title>
    <link rel="stylesheet" href="../smui.css">
    <link rel="stylesheet" href="../system.css">
` + pageStyle + `
</head>
<body>
{{.Header}}
    <main class="container page-content">
`

const pageFoot = `    </main>
` + themeScript + `
</body>
</html>
`

var guideTemplate = htmltpl.Must(htmltpl.New("guide").Parse(`{{define "link"}}{{if .Href}}<a href="{{.Href}}">{{.Name}}</a>{{else}}{{.Name}}{{end}}{{end}}
{{define "upkeep"}}<table><tr><th>Upkeep</th><th class="num">/cycle</th><th class="num">/day</th></tr>{{range .}}<tr><td>{{template "link" .Item}}</td><td class="num">{{.PerCycle}}</td><td class="num">{{.PerDay}}</td></tr>{{end}}</table>{{end}}
{{define "grid"}}power {{if .PowerDeficit}}<span class="deficit">⚠ {{.Power}}</span>{{else}}{{.Power}}{{end}} · life support {{if .LifeSupportDeficit}}<span class="deficit">⚠ {{.LifeSupport}}</span>{{else}}{{.LifeSupport}}{{end}} <span class="text-muted">(supply / draw)</span>{{end}}
` + pageHead + `        <div class="breadcrumb"><a href="index.html">Guides</a> / {{.Title}}</div>
        <h2>{{.Title}}</h2>
        <div class="guide-intro">{{.Intro}}</div>
        <div class="guide-layout">
        <div class="guide-steps">
{{- range .Steps}}
        <section class="step-card" id="step-{{.Num}}">
            <h3>Step {{.Num}}: {{if .Facility}}{{template "link" .Facility}}{{else}}{{.Title}}{{end}}{{if and .Facility (ne .Title .Facility.Name)}} <small class="text-muted">({{.Title}})</small>{{end}}</h3>
{{- if .Gives}}
            <p class="step-gives">{{.Gives}}</p>
{{- end}}
{{- if .Facts}}
            <table class="facts">{{range .Facts}}<tr><td>{{.Label}}</td><td><b>{{.Value}}</b></td><td>{{.Basis}}</td></tr>{{end}}</table>
{{- end}}
{{- if .Facility}}
            <p><b>Build:</b> {{.Credits}} cr{{range .Materials}} + {{.Qty}} {{template "link" .Item}}{{end}}{{if .BuildTime}} · {{.BuildTime}} ticks{{end}}</p>
            <div class="upkeep">{{if .NoUpkeep}}<p class="no-upkeep">No upkeep</p>{{else}}{{template "upkeep" .Upkeep}}{{end}}</div>
{{- end}}
{{- if .Measured}}
            <div class="measured"><b>measured ({{.Measured.Date}})</b>{{if .Measured.Source}} · {{.Measured.Source}}{{end}}{{template "upkeep" .Measured.Upkeep}}</div>
{{- end}}
            {{.Notes}}
{{- if .Alts}}
            <div class="alts"><b>Alternatives:</b>{{range .Alts}}<div>{{template "link" .Facility}}: {{.Credits}} cr · upkeep {{.PerDay}}/day{{if .Gives}} · {{.Gives}}{{end}}</div>{{end}}</div>
{{- end}}
            <div class="so-far">Station so far: {{.SoFarItems}} upkeep items, {{.SoFarDay}} units/day · {{template "grid" .Grid}}</div>
        </section>
{{- end}}
        </div>
        <aside class="guide-sidebar">
            <h3>Station upkeep / day</h3>
            <div class="text-muted">after every step, catalog values</div>
            {{template "upkeep" .Sidebar.Upkeep}}
            <p>{{template "grid" .Sidebar.Grid}}</p>
            <p><b>Build credits:</b> {{.Sidebar.Credits}}</p>
            <details><summary>Build materials</summary><table>{{range .Sidebar.Materials}}<tr><td>{{template "link" .Item}}</td><td class="num">{{.Qty}}</td></tr>{{end}}</table></details>
        </aside>
        </div>
        <div class="guide-body">{{.Body}}</div>
` + pageFoot))

type indexEntry struct {
	Slug, Title, Summary string
}

var indexTemplate = htmltpl.Must(htmltpl.New("index").Parse(pageHead + `        <h2>Guides</h2>
        <p class="text-muted">Hand-written walkthroughs of what the official docs leave thin. Numbers come from the game catalog at build time.</p>
        <ul>
{{- range .Guides}}
            <li><a href="{{.Slug}}.html">{{.Title}}</a>{{if .Summary}} &mdash; {{.Summary}}{{end}}</li>
{{- end}}
        </ul>
` + pageFoot))
