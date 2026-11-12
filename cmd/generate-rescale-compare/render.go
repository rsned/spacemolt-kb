package main

import (
	"bytes"
	"fmt"
	htmltpl "html/template"
	"math"
	"strings"

	"github.com/rsned/spacemolt-kb/internal/kbnav"
)

// bodyColor is the dot color per POI type.
var bodyColor = map[string]string{
	"sun":           "#ffd27a",
	"planet":        "#7fb3ff",
	"station":       "#7ee0a6",
	"asteroid_belt": "#b9a68a",
	"gas_cloud":     "#c79bff",
	"ice_field":     "#a8e6ff",
	"nebula":        "#d9a3ff",
	"relic":         "#ff9f7f",
}

// scaleBar picks a round bar length for a half whose outermost body is at r AU.
func scaleBar(r float64) float64 {
	for _, b := range []float64{0.01, 0.02, 0.05, 0.1, 0.2, 0.5, 1, 2, 5, 10, 20, 50} {
		if b >= r/5 {
			return b
		}
	}
	return 100
}

func fmtAU(v float64) string {
	switch {
	case v == 0:
		return "0"
	case v < 0.1:
		return fmt.Sprintf("%.3f", v)
	case v < 10:
		return fmt.Sprintf("%.2f", v)
	default:
		return fmt.Sprintf("%.1f", v)
	}
}

// half draws one side of the split canvas: orbit rings, one dot per body and a
// scale bar, scaled to fit its own system.
func half(b *strings.Builder, bodies []Body, x0, w, h float64, label string) {
	r := extent(bodies)
	if r == 0 {
		r = 1
	}
	s := (math.Min(w, h)/2 - 22) / (r * 1.06)
	cx, cy := x0+w/2, h/2+8
	fmt.Fprintf(b, `<text x="%.0f" y="20" class="rc-label">%s</text>`, x0+12, htmltpl.HTMLEscapeString(label))
	fmt.Fprintf(b, `<text x="%.0f" y="20" class="rc-note" text-anchor="end">outermost %s AU</text>`, x0+w-12, fmtAU(r))
	for _, p := range bodies {
		if d := math.Hypot(p.X, p.Y); d > 0 {
			fmt.Fprintf(b, `<circle cx="%.1f" cy="%.1f" r="%.1f" class="rc-orbit"/>`, cx, cy, d*s)
		}
	}
	for _, p := range bodies {
		rad := 3.5
		if p.Type == "sun" {
			rad = 7
		}
		col := bodyColor[p.Type]
		if col == "" {
			col = "#cccccc"
		}
		fmt.Fprintf(b, `<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s"><title>%s · (%s, %s) · %s AU from the star</title></circle>`,
			cx+p.X*s, cy+p.Y*s, rad, col, htmltpl.HTMLEscapeString(p.Name), fmtAU(p.X), fmtAU(p.Y), fmtAU(math.Hypot(p.X, p.Y)))
	}
	bar := scaleBar(r)
	fmt.Fprintf(b, `<line x1="%.0f" y1="%.0f" x2="%.1f" y2="%.0f" class="rc-bar"/><text x="%.1f" y="%.0f" class="rc-note">%s AU</text>`,
		x0+14, h-10, x0+14+bar*s, h-10, x0+18+bar*s, h-6, fmtAU(bar))
}

// splitSVG is the before/after canvas, split down the middle.
func splitSVG(s System) htmltpl.HTML {
	const w, h = 880.0, 440.0
	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="rc-canvas" viewBox="0 0 %.0f %.0f" role="img" aria-label="%s before and after the v0.698 rescale">`, w, h, htmltpl.HTMLEscapeString(s.Name))
	half(&b, s.Before, 0, w/2, h, "BEFORE · v0.697 and earlier")
	fmt.Fprintf(&b, `<line x1="%.0f" y1="0" x2="%.0f" y2="%.0f" class="rc-split"/>`, w/2, w/2, h)
	half(&b, s.After, w/2, w/2, h, "AFTER · v0.698 real scale")
	b.WriteString(`</svg>`)
	return htmltpl.HTML(b.String()) //nolint:gosec // built from escaped values above
}

var funcs = htmltpl.FuncMap{
	"au":       fmtAU,
	"splitSVG": splitSVG,
	"ratio": func(r BodyRow) string {
		switch {
		case !r.HasB:
			return "new"
		case !r.HasA:
			return "gone"
		case r.Ratio() == 0, math.Abs(r.After-r.Before) < 0.15:
			// Before-side positions are rounded to 0.1 AU, so a smaller
			// move is rounding, not a change.
			return ""
		}
		return fmt.Sprintf("×%.1f", r.Ratio())
	},
}

const pageHead = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}} - Spacemolt KB</title>
    <link rel="stylesheet" href="../../smui.css">
    <link rel="stylesheet" href="../../missions/missions.css">
    <link rel="stylesheet" href="rescale.css">
</head>
<body>
{{.Header}}
    <main class="container page-content kbm">`

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
    })();
    </script>
</body>
</html>
`

var indexTmpl = htmltpl.Must(htmltpl.New("index").Funcs(funcs).Parse(pageHead + `
        <div class="breadcrumb"><a href="../index.html">Did You Know?</a> / The Great Rescale</div>
        <h2>The Great Rescale <span class="text-muted">v0.698.0 · {{.Rescanned}} of {{len .Systems}} systems rescanned</span></h2>
        <p class="rc-intro">On 2026-11-12 every star system was laid out to fit its star. Small red dwarf systems shrank to well under 1 AU across; systems around bright or giant stars spread out, with belts and ice fields tens of AU from the star. Pick a system to see its layout before and after. Systems agents have not rescanned yet are listed as awaiting rescan.</p>
        <div class="kbm-index">
{{- range .Systems}}
{{- if .Rescanned}}
            <a href="{{.ID}}.html"><span class="n">{{au .BeforeExtent}} → {{au .AfterExtent}} AU</span><span class="t">{{.Name}}</span>{{with .Class}}<span class="d">{{.}}</span>{{end}}</a>
{{- else}}
            <div class="rc-pending"><span class="n">awaiting rescan</span><span class="t">{{.Name}}</span></div>
{{- end}}
{{- end}}
        </div>` + pageFoot))

var detailTmpl = htmltpl.Must(htmltpl.New("detail").Funcs(funcs).Parse(pageHead + `
        <div class="breadcrumb"><a href="../index.html">Did You Know?</a> / <a href="index.html">The Great Rescale</a> / {{.S.Name}}</div>
        <h2>{{.S.Name}} <span class="text-muted">{{with .S.Class}}{{.}} · {{end}}outermost body {{au .S.BeforeExtent}} → {{au .S.AfterExtent}} AU</span></h2>
        <p class="rc-intro"><a href="../../systems/{{.S.ID}}/">Current {{.S.Name}} system page →</a></p>
        {{splitSVG .S}}
        <p class="text-muted rc-intro">Each half is scaled to fit its own system; compare sizes with the scale bars. Hover a dot for its position.</p>
        <table class="kbm-rows rc-table">
            <colgroup><col><col class="c-num"><col class="c-num"><col class="c-num"></colgroup>
            <thead><tr><th>Body</th><th class="r">Before (AU)</th><th class="r">After (AU)</th><th class="r">Change</th></tr></thead>
            <tbody>
{{- range .S.Rows}}
                <tr{{if not .HasA}} class="leg-elsewhere"{{end}}><td>{{.Name}}</td><td class="value">{{if .HasB}}{{au .Before}}{{else}}—{{end}}</td><td class="value">{{if .HasA}}{{au .After}}{{else}}—{{end}}</td><td class="value">{{ratio .}}</td></tr>
{{- end}}
            </tbody>
        </table>` + pageFoot))

const css = `/* The Great Rescale compare pages (generate-rescale-compare). */
.rc-intro { font-size: 13px; margin: 0.4rem 0 0.8rem; }
.rc-canvas { width: 100%; background: #0b0f17; border-radius: 6px; display: block; }
.rc-label { fill: #cfd8e3; font-size: 13px; font-weight: 600; }
.rc-note { fill: #9fb0c2; font-size: 11px; }
.rc-orbit { fill: none; stroke: #ffffff1c; }
.rc-split { stroke: #ffffff44; stroke-dasharray: 4 4; }
.rc-bar { stroke: #cfd8e3; }
.rc-table { margin-top: 0.8rem; }
.rc-table th.r { text-align: right; }
.rc-pending { padding: 0.4rem 0.25rem; border-bottom: 1px solid hsl(var(--border) / 0.6); color: hsl(var(--muted-foreground)); }
.rc-pending .n { float: right; font-size: 12.5px; }
.kbm-index .rc-pending .t { color: inherit; font-weight: 400; }
`

func render(t *htmltpl.Template, data any) ([]byte, error) {
	var b bytes.Buffer
	err := t.Execute(&b, data)
	return b.Bytes(), err
}

func header() htmltpl.HTML { return htmltpl.HTML(kbnav.Header("../../")) } //nolint:gosec // site header, generated internally
