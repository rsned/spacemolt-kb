package main

import (
	"bytes"
	"fmt"
	htmltpl "html/template"
	"math"
	"strconv"
	"strings"

	"github.com/rsned/spacemolt-kb/internal/kbnav"
	"github.com/rsned/spacemolt-kb/pkg/systemmap"
)

// sciShort formats a small ratio as "1.7e-2": one decimal, exponent unpadded.
func sciShort(v float64) string {
	m, e, _ := strings.Cut(fmt.Sprintf("%.1e", v), "e")
	exp, _ := strconv.Atoi(e)
	return m + "e" + strconv.Itoa(exp)
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

// mapPOIs converts a layout's bodies for the system map renderer. IDs come from
// names (the map seeds planet and belt textures from them), so the same body
// looks the same on both sides.
func mapPOIs(bodies []Body) []systemmap.POI {
	out := make([]systemmap.POI, 0, len(bodies))
	for _, b := range bodies {
		out = append(out, systemmap.POI{
			ID: strings.Map(func(r rune) rune {
				if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
					return r
				}
				return '_'
			}, strings.ToLower(b.Name)),
			Name: b.Name, Type: b.Type, Class: b.Class, PositionX: b.X, PositionY: b.Y,
		})
	}
	return out
}

// halfMap renders one side as a full system map (true-size star with haze,
// planets by type, belt and ice particles, labels, linear ruler) without the
// gate ring or legend. fit > 0 forces a shared scale; prefix keeps its SVG ids
// apart from the other maps on the page.
func halfMap(id, name string, bodies []Body, fit float64, prefix string) string {
	sys := &systemmap.System{ID: id, Name: name, POIs: mapPOIs(bodies)}
	svg := systemmap.RenderSystemMapWith(sys, nil, true, systemmap.MapOptions{Linear: true, FitAU: fit, NoGates: true, NoLegend: true})
	return systemmap.PrefixSVGIDs(svg, prefix)
}

// splitSVG is the before/after pair, split down the middle. With same set,
// both sides share one scale (the larger layout) so the size change shows;
// otherwise each side is scaled to fit its own system.
func splitSVG(s System, same bool) htmltpl.HTML {
	fit, view := 0.0, "own"
	if same {
		fit, view = math.Max(extent(s.Before), extent(s.After)), "same"
	}
	var b strings.Builder
	b.WriteString(`<div class="rc-pair">`)
	for _, side := range []struct {
		label, tag string
		bodies     []Body
	}{{"BEFORE · v0.697 and earlier", "b", s.Before}, {"AFTER · v0.698 real scale", "a", s.After}} {
		fmt.Fprintf(&b, `<div class="rc-half"><div class="rc-cap"><span>%s</span><span class="rc-out">outermost %s AU</span></div>%s</div>`,
			htmltpl.HTMLEscapeString(side.label), fmtAU(extent(side.bodies)),
			halfMap(s.ID, s.Name, side.bodies, fit, side.tag+"-"+view+"-"))
	}
	b.WriteString(`</div>`)
	return htmltpl.HTML(b.String()) //nolint:gosec // generated SVG, names escaped by the renderer
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
		if r.Ratio() < 0.1 {
			return "×" + sciShort(r.Ratio())
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
        <div class="rc-views">
            <input type="radio" name="rc-scale" id="rc-own" checked><label for="rc-own">Own scale</label>
            <input type="radio" name="rc-scale" id="rc-same"><label for="rc-same">Same scale</label>
            <div class="rc-view rc-view-own">{{splitSVG .S false}}
                <p class="text-muted rc-intro">Each half is scaled to fit its own system; compare sizes with the scale bars. Hover a dot for its position.</p></div>
            <div class="rc-view rc-view-same">{{splitSVG .S true}}
                <p class="text-muted rc-intro">Both halves share one scale, set by the larger layout, so the change in size shows directly. Hover a dot for its position.</p></div>
        </div>
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
.rc-pair { display: grid; grid-template-columns: 1fr 1fr; gap: 0; background: #1a1e2a; border-radius: 6px; overflow: hidden; }
.rc-half { min-width: 0; }
.rc-half + .rc-half { border-left: 1px dashed #ffffff44; }
.rc-half svg { width: 100%; height: auto; display: block; background: #1a1e2a; }
.rc-cap { display: flex; justify-content: space-between; padding: 0.5rem 0.75rem 0; font-size: 12.5px; font-weight: 600; color: #cfd8e3; }
.rc-cap .rc-out { font-weight: 400; color: #9fb0c2; }
@media (max-width: 640px) {
  .rc-pair { grid-template-columns: 1fr; }
  .rc-half + .rc-half { border-left: 0; border-top: 1px dashed #ffffff44; }
}
.rc-table { margin-top: 0.8rem; }
.rc-views > input { position: absolute; opacity: 0; }
.rc-views > label { display: inline-block; padding: 0.25rem 0.7rem; margin: 0 0.3rem 0.5rem 0; font-size: 12.5px;
  border: 1px solid hsl(var(--border)); border-radius: 999px; cursor: pointer; color: hsl(var(--muted-foreground)); }
.rc-views > input:checked + label { color: hsl(var(--foreground)); border-color: hsl(var(--primary)); background: hsl(var(--accent)); }
.rc-views > input:focus-visible + label { outline: 2px solid hsl(var(--primary)); }
.rc-view { display: none; }
#rc-own:checked ~ .rc-view-own, #rc-same:checked ~ .rc-view-same { display: block; }
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
