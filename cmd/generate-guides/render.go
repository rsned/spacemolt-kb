package main

import (
	"bytes"
	"html"
	htmltpl "html/template"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/rsned/spacemolt-kb/internal/kbnav"
	"github.com/rsned/spacemolt-kb/pkg/catalog"
	"github.com/rsned/spacemolt-kb/pkg/stationplan"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
)

// markdown renders with goldmark's safe defaults (raw HTML in a guide is
// dropped, so the only markup on the page is what the generator emits) plus
// GFM tables, which the reference sections use, and heading ids, which the
// on-this-page TOC links to.
var markdown = goldmark.New(goldmark.WithExtensions(extension.Table),
	goldmark.WithParserOptions(parser.WithAutoHeadingID()))

// bodyH2 matches the reference-section headings goldmark emits, for the TOC;
// tags strips their inline markup.
var (
	bodyH2 = regexp.MustCompile(`<h2 id="([^"]+)">(.*?)</h2>`)
	tags   = regexp.MustCompile(`<[^>]+>`)
)

// renderer turns parsed guides into page view models. warnings collects
// unresolved [[id]] references.
type renderer struct {
	cycleTicks int  // maintenance cycle of the guide being rendered
	perDay     bool // the guide shows per-day figures beside per-cycle ones
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
	Upkeep   string // total upkeep units, "N/cycle" or "N/day"
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
	Break      htmltpl.HTML
	Measured   *measuredView
	Alts       []altView
	SoFarItems int
	SoFar      string // "N units/cycle" or "N units/day"
	Grid       gridView
}

type sidebarView struct {
	Upkeep    []qtyView
	Credits   string
	Materials []qtyView
	Grid      gridView
}

// tocEntry is one on-this-page link; Sub entries (build steps) are indented.
type tocEntry struct {
	ID, Label string
	Sub       bool
}

type pageView struct {
	Cycle   string // e.g. "1 maintenance cycle = 3,000 ticks (≈8.3 h, 2.9/day) · player-reported"
	Unit    string // "cycle" or "day": what the upkeep panel totals are per
	Header  htmltpl.HTML
	Title   string
	Summary string
	TOC     []tocEntry
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
			v.PerCycle = commas(q)
			if r.perDay {
				v.PerDay = perDayText(stationplan.PerDay(q, r.cycleTicks))
			}
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

// upkeepTotal is the total upkeep units in m, per day when the guide shows
// per-day figures and per cycle otherwise, with its unit suffix.
func (r *renderer) upkeepTotal(m map[string]int) string {
	if r.perDay {
		return perDayText(r.perDayTotal(m)) + "/day"
	}
	n := 0
	for _, q := range m {
		n += q
	}
	return commas(n) + "/cycle"
}

func (r *renderer) perDayTotal(m map[string]int) float64 {
	n := 0.0
	for _, q := range m {
		n += stationplan.PerDay(q, r.cycleTicks)
	}
	return n
}

// perDayText shows small daily quantities to one decimal (2.9, not 3) and
// large ones as grouped integers.
func perDayText(f float64) string {
	if f < 100 {
		return strconv.FormatFloat(f, 'f', 1, 64)
	}
	return commas(int(f + 0.5))
}

// page builds the view for g given its cumulative totals (one per step).
func (r *renderer) page(g *guide, totals []stationplan.Totals) (pageView, error) {
	r.cycleTicks = g.MaintenanceCycleTicks
	r.perDay = g.ShowPerDay
	pv := pageView{Header: htmltpl.HTML(kbnav.Header("../")), Title: g.Title, //nolint:gosec // site header, generated internally
		Summary: strings.TrimSpace(g.Summary), Cycle: cycleNote(g.MaintenanceCycleTicks, g.MaintenanceCycleBasis, g.ShowPerDay), Unit: "cycle"}
	if g.ShowPerDay {
		pv.Unit = "day"
	}
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
			SoFarItems: len(t.UpkeepPerCycle), SoFar: strings.Replace(r.upkeepTotal(t.UpkeepPerCycle), "/", " units/", 1)}
		if sv.Notes, err = r.md(g, s.Notes); err != nil {
			return pv, err
		}
		if sv.Break, err = r.md(g, s.BreakBefore); err != nil {
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
				Upkeep: r.upkeepTotal(upkeepMap(f)), Gives: gives(f)})
		}
		pv.Steps = append(pv.Steps, sv)
		label := s.Title
		if sv.Facility != nil {
			label = sv.Facility.Name
		}
		pv.TOC = append(pv.TOC, tocEntry{ID: "step-" + strconv.Itoa(i), Label: "Step " + strconv.Itoa(i) + ": " + label, Sub: true})
	}
	for _, m := range bodyH2.FindAllStringSubmatch(string(pv.Body), -1) {
		pv.TOC = append(pv.TOC, tocEntry{ID: m[1], Label: html.UnescapeString(tags.ReplaceAllString(m[2], ""))})
	}
	if n := len(totals); n > 0 {
		last := totals[n-1]
		pv.Sidebar = sidebarView{Upkeep: r.qtys(last.UpkeepPerCycle, true), Credits: commas(last.Credits),
			Materials: r.qtys(last.Materials, false), Grid: grid(last)}
	}
	return pv, nil
}

// cycleNote states the maintenance cycle length and its basis, with cycles
// per day only when the guide shows per-day figures.
func cycleNote(ticks int, basis string, perDay bool) string {
	hours := float64(ticks*stationplan.TickSeconds) / 3600
	note := "1 maintenance cycle = " + commas(ticks) + " ticks (≈" + strconv.FormatFloat(hours, 'f', 1, 64) + " h"
	if perDay {
		note += ", " + strconv.FormatFloat(stationplan.CyclesPerDay(ticks), 'f', 1, 64) + "/day"
	}
	note += ")"
	if basis != "" {
		note += " · " + basis
	}
	return note
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

// pageStyle is the guide look: a left-edge TOC, a wide reading column with
// sans-serif prose (monospace kept for headings, numbers and commands), roomy
// step cards, and station upkeep in a drawer that slides over the page.
// smui.css colour tokens are bare HSL triples, so they are wrapped in hsl().
const pageStyle = `<style>
        html { scroll-behavior: smooth; scroll-padding-top: 24px; overflow-x: clip; }
        .guide-shell { display: grid; grid-template-columns: 260px minmax(0, 72rem); gap: 56px; padding: 32px 48px 96px 24px; align-items: start; }
        .guide-toc { position: sticky; top: 24px; max-height: calc(100vh - 48px); overflow-y: auto; font-size: 14px; line-height: 1.4; border-left: 1px solid hsl(var(--border)); }
        .guide-toc .toc-title, .guide-toc .toc-group { font-size: 12px; letter-spacing: .08em; text-transform: uppercase; color: hsl(var(--muted-foreground)); margin: 0 0 12px 16px; }
        .guide-toc .toc-group { margin: 18px 0 6px 16px; }
        .guide-toc a { display: block; padding: 6px 12px 6px 16px; margin-left: -1px; border-left: 2px solid transparent; color: hsl(var(--muted-foreground)); text-decoration: none; }
        .guide-toc a.sub { padding-left: 28px; }
        .guide-toc a:hover { color: hsl(var(--foreground)); }
        .guide-toc a.active { color: hsl(var(--foreground)); border-left-color: hsl(var(--smui-yellow)); font-weight: 600; }

        .guide-main { min-width: 0; font-family: 'Inter', system-ui, -apple-system, 'Segoe UI', sans-serif; font-size: 18px; line-height: 1.75; overflow-wrap: anywhere; }
        .guide-main h1, .guide-main h2, .guide-main h3, .guide-main code, .guide-main .num, .guide-main .facts b, .guide-main .breadcrumb { font-family: var(--font-sans); }
        .guide-main p, .guide-main ul, .guide-main ol, .guide-main blockquote, .guide-main table, .guide-main dl, .guide-main details { margin: 0 0 1.15em; }
        .guide-main ul, .guide-main ol { padding-left: 1.4em; }
        .guide-main li { margin: .35em 0; }
        .guide-main .breadcrumb { font-size: 14px; margin-bottom: 1em; }
        .guide-title { font-size: 2.35rem; line-height: 1.2; margin: 0 0 .4em; }
        .guide-lede { font-size: 1.1em; color: hsl(var(--muted-foreground)); margin-bottom: 2em !important; }
        .guide-main h2 { font-size: 1.75rem; line-height: 1.3; margin: 3.2em 0 .9em; padding-top: 1.2em; border-top: 1px solid hsl(var(--border)); }
        .guide-main h3 { font-size: 1.3rem; line-height: 1.35; margin: 2em 0 .6em; }
        .guide-main blockquote { border-left: 4px solid hsl(var(--smui-yellow)); background: hsl(var(--card)); padding: 18px 22px; border-radius: 0 8px 8px 0; }
        .guide-main blockquote p:last-child { margin-bottom: 0; }
        .guide-main code { font-size: .88em; padding: .1em .35em; border-radius: 4px; background: hsl(var(--muted)); }

        .step-card { border: 1px solid hsl(var(--border)); border-radius: 12px; padding: 28px 32px; margin: 0 0 40px; background: hsl(var(--card)); }
        .step-card h3 { margin: 0 0 .25em; font-size: 1.45rem; }
        .step-card h3 small { display: block; font-size: .7em; font-weight: 400; margin-top: .2em; }
        .step-gives { color: hsl(var(--muted-foreground)); font-size: .9em; margin-bottom: 1.4em !important; }
        .step-build { background: hsl(var(--muted) / .5); padding: 10px 14px; border-radius: 8px; }
        .facts { margin: 1.2em 0 1.6em; border-top: 1px solid hsl(var(--border)); }
        .fact { display: grid; grid-template-columns: 11rem minmax(0, 1fr); gap: 20px; padding: 12px 0; border-bottom: 1px solid hsl(var(--border)); }
        .fact dt { color: hsl(var(--muted-foreground)); font-size: .9em; white-space: nowrap; }
        .fact dd b { font-weight: 600; }
        .fact .basis { display: block; font-size: .82em; color: hsl(var(--muted-foreground)); margin-top: 2px; line-height: 1.5; }
        .upkeep, .measured { border-left: 4px solid hsl(var(--smui-yellow)); background: hsl(var(--smui-yellow) / .06); padding: 14px 18px; border-radius: 0 8px 8px 0; margin: 0 0 1.4em; }
        .measured { border-left-color: hsl(var(--smui-green)); background: hsl(var(--smui-green) / .06); font-size: .9em; }
        .upkeep table, .measured table { width: 100%; margin: 0; border-collapse: collapse; font-size: 16px; }
        .upkeep th, .measured th { font-size: 12px; letter-spacing: .06em; text-transform: uppercase; color: hsl(var(--muted-foreground)); font-weight: 500; }
        .upkeep td, .upkeep th, .measured td, .measured th { padding: 6px 0; text-align: left; }
        .upkeep tr + tr td, .measured tr + tr td { border-top: 1px solid hsl(var(--border) / .6); }
        .no-upkeep { color: hsl(var(--muted-foreground)); font-style: italic; margin: 0 !important; }
        .num { text-align: right !important; font-variant-numeric: tabular-nums; width: 5.5rem; }
        .step-break { margin: 2.4em 0 1.6em; padding: 1em 0 0; border-top: 2px dashed hsl(var(--border)); color: hsl(var(--muted-foreground)); }
        .step-break p { margin: 0 0 .5em; }
        .alts { font-size: .9em; color: hsl(var(--muted-foreground)); margin: 0 0 1.4em; line-height: 1.6; }
        .alts > div { padding: 4px 0; }
        .so-far { border-top: 1px dashed hsl(var(--border)); margin-top: 1.6em; padding-top: 14px; font-size: .82em; color: hsl(var(--muted-foreground)); cursor: pointer; }
        .so-far:hover { color: hsl(var(--foreground)); }
        .deficit { color: hsl(var(--smui-red)); font-weight: 600; }

        .guide-body table { width: 100%; border-collapse: collapse; font-size: .9em; line-height: 1.55; }
        .guide-body th { text-align: left; font-size: 13px; letter-spacing: .05em; text-transform: uppercase; color: hsl(var(--muted-foreground)); font-weight: 500; padding: 10px 12px; border-bottom: 2px solid hsl(var(--border)); }
        .guide-body td { padding: 12px; vertical-align: top; border-bottom: 1px solid hsl(var(--border)); }
        .guide-body td:first-child { min-width: 9rem; }
        .guide-body .table-scroll { overflow-x: auto; }

        .guide-drawer { position: fixed; top: 0; right: 0; height: 100vh; width: 360px; max-width: 92vw; overflow-y: auto; z-index: 40; font-size: 14px; line-height: 1.5;
            border-left: 1px solid hsl(var(--border)); padding: 28px 24px 80px; background: hsl(var(--card)); box-shadow: -12px 0 40px rgb(0 0 0 / .18);
            transform: translateX(105%); transition: transform .25s ease; }
        body.drawer-open .guide-drawer { transform: none; }
        .guide-drawer h3 { font-size: 16px; margin: 0 0 6px; }
        .guide-drawer > .text-muted { font-size: 13px; margin-bottom: 14px; }
        .guide-drawer table { width: 100%; border-collapse: collapse; margin: 0 0 14px; }
        .guide-drawer td, .guide-drawer th { padding: 5px 0; text-align: left; vertical-align: top; }
        .guide-drawer tr + tr td { border-top: 1px solid hsl(var(--border) / .6); }
        .guide-drawer .num { width: 3.5rem; }
        .guide-drawer p { margin: 0 0 10px; }
        .drawer-close { position: absolute; top: 14px; right: 14px; border: 0; background: none; font-size: 22px; cursor: pointer; color: hsl(var(--muted-foreground)); }
        .drawer-tab { position: fixed; right: 0; top: 50%; z-index: 45; transform: translateY(-50%); writing-mode: vertical-rl; border: 1px solid hsl(var(--border)); border-right: 0;
            border-radius: 10px 0 0 10px; background: hsl(var(--smui-yellow)); color: #111; padding: 16px 9px; cursor: pointer; font: 600 12px var(--font-sans);
            letter-spacing: .06em; text-transform: uppercase; box-shadow: -4px 4px 16px rgb(0 0 0 / .15); transition: right .25s ease; }
        body.drawer-open .drawer-tab { right: min(360px, 92vw); }
        body.nav-open .drawer-tab { visibility: hidden; }

        @media (max-width: 1000px) { .guide-shell { grid-template-columns: 200px minmax(0, 1fr); gap: 32px; } }
        @media (max-width: 800px) {
            .guide-shell { grid-template-columns: 1fr; padding: 16px; }
            .guide-toc { position: static; max-height: none; }
            .guide-title { font-size: 1.75rem; overflow-wrap: normal; }
            .fact { grid-template-columns: 1fr; gap: 2px; }
            .step-card { padding: 20px 18px; }
            .guide-body table { display: block; overflow-x: auto; }
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

// guideScript opens and closes the upkeep drawer (tab, any "Station so far"
// line, ×, Esc) and highlights the TOC entry for the section being read.
const guideScript = `    <script>
    (function() {
        var drawer = document.getElementById('upkeep-drawer');
        var tab = document.querySelector('.drawer-tab');
        function setOpen(open) {
            document.body.classList.toggle('drawer-open', open);
            if (tab) tab.setAttribute('aria-expanded', open ? 'true' : 'false');
        }
        if (drawer) {
            tab.addEventListener('click', function() { setOpen(!document.body.classList.contains('drawer-open')); });
            drawer.querySelector('.drawer-close').addEventListener('click', function() { setOpen(false); });
            document.querySelectorAll('.so-far').forEach(function(el) { el.addEventListener('click', function() { setOpen(true); }); });
            document.addEventListener('keydown', function(e) { if (e.key === 'Escape') setOpen(false); });
        }
        var links = Array.prototype.slice.call(document.querySelectorAll('.guide-toc a'));
        var targets = links.map(function(a) { return document.getElementById(a.getAttribute('href').slice(1)); });
        function spy() {
            var current = 0;
            targets.forEach(function(t, i) { if (t && t.getBoundingClientRect().top < 120) current = i; });
            links.forEach(function(a, i) { a.classList.toggle('active', i === current); });
        }
        window.addEventListener('scroll', spy, {passive: true});
        spy();
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
    <link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600&display=swap">
` + pageStyle + `
</head>
<body>
{{.Header}}
`

const pageFoot = themeScript + `
</body>
</html>
`

var guideTemplate = htmltpl.Must(htmltpl.New("guide").Parse(`{{define "link"}}{{if .Href}}<a href="{{.Href}}">{{.Name}}</a>{{else}}{{.Name}}{{end}}{{end}}
{{define "upkeep"}}<table><tr><th>Upkeep</th><th class="num">/cycle</th>{{if .}}{{if (index . 0).PerDay}}<th class="num">/day</th>{{end}}{{end}}</tr>{{range .}}<tr><td>{{template "link" .Item}}</td><td class="num">{{.PerCycle}}</td>{{if .PerDay}}<td class="num">{{.PerDay}}</td>{{end}}</tr>{{end}}</table>{{end}}
{{define "grid"}}power {{if .PowerDeficit}}<span class="deficit">⚠ {{.Power}}</span>{{else}}{{.Power}}{{end}} · life support {{if .LifeSupportDeficit}}<span class="deficit">⚠ {{.LifeSupport}}</span>{{else}}{{.LifeSupport}}{{end}} <span class="text-muted">(supply / draw)</span>{{end}}
` + pageHead + `    <main class="guide-shell">
        <nav class="guide-toc" aria-label="On this page">
            <div class="toc-title">On this page</div>
            <a href="#top">Introduction</a>
{{- if .Steps}}
            <div class="toc-group">Build order</div>
{{- end}}
{{- range .TOC}}
            <a {{if .Sub}}class="sub" {{end}}href="#{{.ID}}">{{.Label}}</a>
{{- end}}
        </nav>
        <article class="guide-main">
        <div class="breadcrumb"><a href="index.html">Guides</a> / {{.Title}}</div>
        <h1 class="guide-title" id="top">{{.Title}}</h1>
{{- if .Summary}}
        <p class="guide-lede">{{.Summary}}</p>
{{- end}}
        <div class="guide-intro">{{.Intro}}</div>
{{- if .Steps}}
        <h2 id="build-order">Build order</h2>
        <div class="guide-steps">
{{- range .Steps}}
{{- if .Break}}
        <div class="step-break">{{.Break}}</div>
{{- end}}
        <section class="step-card" id="step-{{.Num}}">
            <h3>Step {{.Num}}: {{if .Facility}}{{template "link" .Facility}}{{else}}{{.Title}}{{end}}{{if and .Facility (ne .Title .Facility.Name)}} <small class="text-muted">{{.Title}}</small>{{end}}</h3>
{{- if .Gives}}
            <p class="step-gives">{{.Gives}}</p>
{{- end}}
{{- if .Facts}}
            <dl class="facts">{{range .Facts}}<div class="fact"><dt>{{.Label}}</dt><dd><b>{{.Value}}</b>{{if .Basis}}<span class="basis">{{.Basis}}</span>{{end}}</dd></div>{{end}}</dl>
{{- end}}
{{- if .Facility}}
            <p class="step-build"><b>Build:</b> {{.Credits}} cr{{range .Materials}} + {{.Qty}} {{template "link" .Item}}{{end}}{{if .BuildTime}} · {{.BuildTime}} ticks{{end}}</p>
            <div class="upkeep">{{if .NoUpkeep}}<p class="no-upkeep">No upkeep</p>{{else}}{{template "upkeep" .Upkeep}}{{end}}</div>
{{- end}}
{{- if .Measured}}
            <div class="measured"><b>measured ({{.Measured.Date}})</b>{{if .Measured.Source}} · {{.Measured.Source}}{{end}}{{template "upkeep" .Measured.Upkeep}}</div>
{{- end}}
            {{.Notes}}
{{- if .Alts}}
            <div class="alts"><b>Alternatives:</b>{{range .Alts}}<div>{{template "link" .Facility}}: {{.Credits}} cr · upkeep {{.Upkeep}}{{if .Gives}} · {{.Gives}}{{end}}</div>{{end}}</div>
{{- end}}
            <div class="so-far" title="Open station upkeep">Station so far: {{.SoFarItems}} upkeep items, {{.SoFar}} · {{template "grid" .Grid}}</div>
        </section>
{{- end}}
        </div>
{{- end}}
        <div class="guide-body">{{.Body}}</div>
        </article>
    </main>
{{- if .Steps}}
    <aside class="guide-sidebar guide-drawer" id="upkeep-drawer" aria-label="Station upkeep">
        <button class="drawer-close" type="button" aria-label="Close">×</button>
        <h3>Station upkeep / {{.Unit}}</h3>
        <div class="text-muted">after every step, catalog values; {{.Cycle}}</div>
        {{template "upkeep" .Sidebar.Upkeep}}
        <p>{{template "grid" .Sidebar.Grid}}</p>
        <p><b>Build credits:</b> {{.Sidebar.Credits}}</p>
        <details><summary>Build materials</summary><table>{{range .Sidebar.Materials}}<tr><td>{{template "link" .Item}}</td><td class="num">{{.Qty}}</td></tr>{{end}}</table></details>
    </aside>
    <button class="drawer-tab" type="button" aria-controls="upkeep-drawer" aria-expanded="false">Station upkeep</button>
{{- end}}
` + guideScript + `
` + pageFoot))

type indexEntry struct {
	Slug, Title, Summary string
}

var indexTemplate = htmltpl.Must(htmltpl.New("index").Parse(pageHead + `    <main class="container page-content">
        <h2>Guides</h2>
        <p class="text-muted">Hand-written walkthroughs of what the official docs leave thin. Numbers come from the game catalog at build time.</p>
        <ul>
{{- range .Guides}}
            <li><a href="{{.Slug}}.html">{{.Title}}</a>{{if .Summary}} &mdash; {{.Summary}}{{end}}</li>
{{- end}}
        </ul>
    </main>
` + pageFoot))
