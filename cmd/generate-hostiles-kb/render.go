package main

import (
	"bytes"
	"fmt"
	htmltpl "html/template"
	"strings"

	humanize "github.com/dustin/go-humanize"

	"github.com/rsned/spacemolt-kb/internal/kbnav"
)

// Page is one hostiles list page (Frontier or Pirates).
type Page struct {
	Title, Intro, Empty string
	Types               []*HostileType
}

// Units totals the units seen across the page's types.
func (p Page) Units() int {
	n := 0
	for _, t := range p.Types {
		n += t.Units
	}
	return n
}

// Systems counts the distinct systems any type was seen in.
func (p Page) Systems() int {
	seen := map[string]bool{}
	for _, t := range p.Types {
		for _, s := range t.Systems {
			seen[s.SystemID] = true
		}
	}
	return len(seen)
}

// Battles is the most battles logged for any one type on the page.
func (p Page) Battles() int {
	n := 0
	for _, t := range p.Types {
		n = max(n, t.Battles)
	}
	return n
}

var funcs = htmltpl.FuncMap{
	// dash renders a count, or "—" when it was never captured.
	"dash": func(n int) htmltpl.HTML {
		if n == 0 {
			return `<span class="text-muted">—</span>`
		}
		return htmltpl.HTML(humanize.Comma(int64(n))) //nolint:gosec // digits only
	},
	"dmg": func(v float64) string {
		if v == float64(int64(v)) {
			return humanize.Comma(int64(v))
		}
		return fmt.Sprintf("%.1f", v)
	},
	"when": func(s string) string { // 2026-11-12T22:14:12Z → 2026-11-12 22:14
		if len(s) >= 16 {
			return strings.Replace(s[:16], "T", " ", 1)
		}
		return s
	},
	"title": func(s string) string {
		if s == "" {
			return ""
		}
		return strings.ToUpper(s[:1]) + s[1:]
	},
}

const pageHead = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}} - Hostiles - Spacemolt KB</title>
    <link rel="stylesheet" href="../smui.css">
    <link rel="stylesheet" href="../system.css">
    <style>
    .hz-summary { display: flex; flex-wrap: wrap; gap: 0.75rem; margin: 1rem 0; }
    .hz-summary > div { padding: 0.6rem 0.9rem; border: 1px solid hsl(var(--border)); border-radius: 6px; min-width: 7rem; }
    .hz-summary .num { font-size: 20px; font-weight: 600; }
    .hz-summary .label { font-size: var(--text-label); color: hsl(var(--muted-foreground)); text-transform: uppercase; letter-spacing: 1px; }
    td.r, th.r { text-align: right; white-space: nowrap; }
    .hz-section { margin-top: 1.5rem; }
    .hz-pick { margin: 1.5rem 0 0.5rem; }
    .hz-empty { padding: 1rem; border: 1px dashed hsl(var(--border)); color: hsl(var(--muted-foreground)); }
    </style>
</head>
<body>
{{.Header}}
    <main class="container page-content">`

const pageFoot = `
    </main>
    <script>
    (function() {
        var toggle = document.getElementById('theme-toggle');
        var root = document.documentElement;
        if (localStorage.getItem('theme') === 'dark') root.classList.add('dark');
        if (toggle) toggle.addEventListener('click', function() {
            root.classList.toggle('dark');
            localStorage.setItem('theme', root.classList.contains('dark') ? 'dark' : 'light');
        });
        // Type selector: show one type's section, kept in the URL hash.
        var sel = document.getElementById('hz-type');
        if (!sel) return;
        function show(slug) {
            document.querySelectorAll('.hz-type').forEach(function(s) { s.hidden = (s.id !== slug); });
            sel.value = slug;
        }
        sel.addEventListener('change', function() { show(sel.value); history.replaceState(null, '', '#' + sel.value); });
        var h = location.hash.slice(1);
        if (h && document.getElementById(h)) show(h);
    })();
    </script>
</body>
</html>
`

var listTmpl = htmltpl.Must(htmltpl.New("list").Funcs(funcs).Parse(pageHead + `
        <div class="breadcrumb"><a href="index.html">Hostiles</a> / {{.P.Title}}</div>
        <h2>{{.P.Title}}</h2>
        <p class="text-muted mt-1">{{.P.Intro}}</p>
{{- if not .P.Types}}
        <div class="hz-empty">{{.P.Empty}}</div>
{{- else}}
        <div class="hz-summary">
            <div><div class="num">{{len .P.Types}}</div><div class="label">Types</div></div>
            <div><div class="num">{{dash .P.Units}}</div><div class="label">Units seen</div></div>
            <div><div class="num">{{dash .P.Systems}}</div><div class="label">Systems</div></div>
            <div><div class="num">{{dash .P.Battles}}</div><div class="label">Battles logged</div></div>
        </div>

        <div class="section-label">Types, most dangerous first</div>
        <table>
            <thead><tr><th>Type</th><th class="r">Max hull</th><th class="r">Max shield</th><th class="r">Units seen</th><th class="r">Systems</th><th class="r">Battles</th><th class="r">Hardest hit</th></tr></thead>
            <tbody>
{{- range .P.Types}}
                <tr><td><a href="#{{.Slug}}" class="hz-jump" data-slug="{{.Slug}}">{{.Name}}</a></td><td class="r">{{dash .MaxHull}}</td><td class="r">{{dash .MaxShield}}</td><td class="r">{{dash .Units}}</td><td class="r">{{dash (len .Systems)}}</td><td class="r">{{dash .Battles}}</td><td class="r">{{if .HardestHit}}{{dmg .HardestHit}}{{else}}<span class="text-muted">—</span>{{end}}</td></tr>
{{- end}}
            </tbody>
        </table>
        <p class="text-muted" style="font-size: 12.5px">Ordered by max hull + shield; types whose hull and shield are not captured yet follow by their hardest hit. "—" means not recorded yet.</p>

        <div class="hz-pick"><label for="hz-type" class="section-label">Type </label>
            <select id="hz-type">
{{- range .P.Types}}
                <option value="{{.Slug}}">{{.Name}}</option>
{{- end}}
            </select>
        </div>
{{- range $i, $t := .P.Types}}
        <section id="{{.Slug}}" class="hz-type"{{if $i}} hidden{{end}}>
            <h3>{{.Name}}</h3>
            <p class="text-muted">{{if .FirstSeen}}First seen {{when .FirstSeen}}, last seen {{when .LastSeen}}.{{else}}Known from battle logs only.{{end}}</p>

            <div class="hz-section section-label">Sightings by system</div>
{{- if .Systems}}
            <table>
                <thead><tr><th>System</th><th class="r">Units</th><th class="r">Most at once</th><th class="r">Sightings</th><th class="r">In battle</th><th class="r">First seen</th><th class="r">Last seen</th></tr></thead>
                <tbody>
{{- range .Systems}}
                    <tr><td><a href="../systems/{{.SystemID}}/">{{.SystemName}}</a></td><td class="r">{{dash .Units}}</td><td class="r">{{dash .MaxAtOnce}}</td><td class="r">{{dash .Sightings}}</td><td class="r">{{.BattlePct}}%</td><td class="r">{{when .FirstSeen}}</td><td class="r">{{when .LastSeen}}</td></tr>
{{- end}}
                </tbody>
            </table>
{{- else}}
            <div class="hz-empty">No sightings recorded yet.</div>
{{- end}}

            <div class="hz-section section-label">Weapons</div>
{{- if .Weapons}}
            <table>
                <thead><tr><th>Weapon</th><th>Damage type</th><th>Shot</th><th class="r">Hit rate</th><th class="r">Damage per hit</th><th class="r">Total damage</th><th class="r">Battles</th></tr></thead>
                <tbody>
{{- range .Weapons}}
                    <tr><td>{{if .Name}}{{.Name}}{{else}}<span class="text-muted">(effect)</span>{{end}}</td><td>{{title .DamageType}}</td><td>{{title .ShotKind}}</td><td class="r">{{.HitPct}}% <span class="text-muted">({{.Hits}}/{{.Shots}})</span></td><td class="r">{{dmg .Min}}–{{dmg .Max}}</td><td class="r">{{dmg .Total}}</td><td class="r">{{.Battles}}</td></tr>
{{- end}}
                </tbody>
            </table>
{{- else}}
            <div class="hz-empty">No battles logged yet.</div>
{{- end}}
        </section>
{{- end}}
        <script>
        document.querySelectorAll('.hz-jump').forEach(function(a) {
            a.addEventListener('click', function(e) { e.preventDefault(); var s = document.getElementById('hz-type'); s.value = a.dataset.slug; s.dispatchEvent(new Event('change')); document.getElementById(a.dataset.slug).scrollIntoView(); });
        });
        </script>
{{- end}}` + pageFoot))

var indexTmpl = htmltpl.Must(htmltpl.New("index").Funcs(funcs).Parse(pageHead + `
        <h2>Hostiles</h2>
        <p class="text-muted mt-1">Hostile NPCs agents have met: where they are seen, how tough they are, and what they hit with. Creatures have their own <a href="../wildlife/index.html">Wildlife</a> section.</p>
        <div class="card" style="padding: 12px 16px; margin-top: 1rem">
            <p><a href="frontier.html">&#x2726; Frontier &mdash; repeaters in uncharted frontier space &rarr;</a> <span class="text-muted">{{.Frontier}}</span></p>
            <p><a href="pirates.html">&#x2620; Pirates &mdash; pirate stronghold crews &rarr;</a> <span class="text-muted">{{.Pirates}}</span></p>
        </div>` + pageFoot))

func render(t *htmltpl.Template, data any) ([]byte, error) {
	var b bytes.Buffer
	err := t.Execute(&b, data)
	return b.Bytes(), err
}

func header() htmltpl.HTML { return htmltpl.HTML(kbnav.Header("../")) } //nolint:gosec // site header, generated internally

// summary is the one-line count on the landing page.
func summary(p Page) string {
	if len(p.Types) == 0 {
		return "nothing recorded yet"
	}
	return fmt.Sprintf("%d %s · %s units seen", len(p.Types), plural(len(p.Types), "type", "types"), humanize.Comma(int64(p.Units())))
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
