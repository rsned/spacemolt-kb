package resourcediff

import (
	"bytes"
	"fmt"
	htmltpl "html/template"
	"time"

	"github.com/rsned/spacemolt-kb/internal/kbnav"
)

// DayReport is everything one report page needs.
type DayReport struct {
	Snapshot *Snapshot
	PrevDate string // previous report date, "" if none
	NextDate string

	// VsPrevious compares against the previous snapshot; nil for the first.
	VsPrevious *Comparison
	// VsBaseline compares against the snapshot taken at the last server
	// content update; nil when this snapshot IS the baseline.
	VsBaseline   *Comparison
	IsBaseline   bool
	BaselineDate string // snapshot date of the baseline

	// The server content patch the baseline tracks. This is the patch that
	// changed resource content (e.g. "11 newly available mineable
	// resources"), which is usually older than the version the snapshot's
	// scrape ran against.
	ContentVersion     string
	ContentReleaseDate string   // release date of that patch, if known
	ContentNotes       []string // its resource-related patch notes
	BaselineReason     string   // why that snapshot is the baseline
	CatalogDiffURL     string   // link to the kb/diffs report covering that patch, if one exists
}

// IndexEntry is one row of the changes index.
type IndexEntry struct {
	Date           string
	ServerVersion  string
	IsBaseline     bool
	ContentVersion string // set on baseline rows
	VsPrevious     string // summary line
	VsBaseline     string // summary line, "" for the baseline row
}

var funcs = htmltpl.FuncMap{
	"formatDate": func(s string) string {
		if t, err := time.Parse("2006-01-02", s); err == nil {
			return t.Format("January 2, 2006")
		}
		return s
	},
	"delta": func(oldV, newV int) htmltpl.HTML {
		switch {
		case newV > oldV:
			return htmltpl.HTML(fmt.Sprintf(`<span class="diff-add">+%d</span>`, newV-oldV))
		case newV < oldV:
			return htmltpl.HTML(fmt.Sprintf(`<span class="diff-del">&minus;%d</span>`, oldV-newV))
		}
		return `<span class="text-muted">&plusmn;0</span>`
	},
	"itemURL": func(t ResourceType) string {
		if t.Category == "" {
			return ""
		}
		return "../../items/" + t.Category + "/" + t.ID + ".html"
	},
	"plural": func(n int, s string) string {
		if n == 1 {
			return s
		}
		return s + "s"
	},
	"tick": func(t int) string {
		if t == 0 {
			return "-"
		}
		return fmt.Sprint(t)
	},
}

// RenderDayReport renders one report page.
func RenderDayReport(r DayReport) (string, error) {
	t, err := htmltpl.New("day").Funcs(funcs).Parse(dayTemplate)
	if err != nil {
		return "", fmt.Errorf("parse day template: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, struct {
		DayReport
		Header htmltpl.HTML
	}{r, htmltpl.HTML(kbnav.Header("../../"))}); err != nil { //nolint:gosec // site header, generated internally
		return "", fmt.Errorf("execute day template: %w", err)
	}
	return buf.String(), nil
}

// RenderIndex renders the changes index page, newest first.
func RenderIndex(entries []IndexEntry) (string, error) {
	t, err := htmltpl.New("index").Funcs(funcs).Parse(indexTemplate)
	if err != nil {
		return "", fmt.Errorf("parse index template: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, struct {
		Entries []IndexEntry
		Header  htmltpl.HTML
	}{entries, htmltpl.HTML(kbnav.Header("../../"))}); err != nil { //nolint:gosec // site header, generated internally
		return "", fmt.Errorf("execute index template: %w", err)
	}
	return buf.String(), nil
}

const pageStyle = `<style>
.diff-nav { display: flex; justify-content: space-between; padding: 1rem 0; font-size: var(--text-ui); }
.diff-nav a { color: hsl(var(--primary)); }
.diff-summary { padding: 0.5rem 0 1rem; color: hsl(var(--muted-foreground)); font-size: var(--text-ui); }
.cmp-section { border: 1px solid hsl(var(--border)); margin: 1.5rem 0; }
.cmp-header { padding: 0.75rem 1rem; background: hsl(var(--card)); border-bottom: 1px solid hsl(var(--border)); }
.cmp-header h3 { font-size: var(--text-ui); font-weight: 600; margin: 0; }
.cmp-header p { margin: 0.25rem 0 0; font-size: var(--text-label); color: hsl(var(--muted-foreground)); }
.cmp-body { padding: 1rem; }
.cmp-body h4 { font-size: var(--text-label); text-transform: uppercase; letter-spacing: 1.5px; color: hsl(var(--muted-foreground)); margin: 1.25rem 0 0.5rem; }
.cmp-body h4:first-child { margin-top: 0; }
.cmp-body h5 { font-size: var(--text-ui); font-weight: 600; margin: 0.75rem 0 0.25rem; }
.diff-add { color: hsl(var(--smui-green)); }
.diff-del { color: hsl(var(--smui-red)); }
.diff-mod { color: hsl(var(--smui-yellow)); }
.diff-list { list-style: none; padding: 0; margin: 0; }
.diff-list li { padding: 0.2rem 0; font-size: var(--text-ui); }
.diff-list a, .cmp-body table a { color: inherit; text-decoration: underline; text-underline-offset: 2px; }
.diff-list a:hover, .cmp-body table a:hover { color: hsl(var(--primary)); }
.no-changes { color: hsl(var(--muted-foreground)); font-style: italic; }
.cmp-body table { width: 100%; border-collapse: collapse; font-size: var(--text-ui); margin: 0.25rem 0 0.5rem; }
.cmp-body th { text-align: left; padding: 0.25rem 0.5rem; color: hsl(var(--muted-foreground)); font-weight: 600; font-size: var(--text-label); border-bottom: 1px solid hsl(var(--border)); }
.cmp-body td { padding: 0.2rem 0.5rem; vertical-align: top; border-bottom: 1px solid hsl(var(--border) / 0.3); }
.cmp-body td.num { text-align: right; font-variant-numeric: tabular-nums; }
.stat-row { display: flex; flex-wrap: wrap; gap: 1.5rem; font-size: var(--text-ui); margin-bottom: 0.5rem; }
.stat-row span b { font-weight: 600; }
.badge-hidden { font-size: var(--text-label); color: hsl(var(--smui-yellow)); border: 1px solid hsl(var(--smui-yellow)); border-radius: 3px; padding: 0 0.3rem; }
.baseline-tag { font-size: var(--text-label); color: hsl(var(--primary)); border: 1px solid hsl(var(--primary)); border-radius: 3px; padding: 0 0.3rem; }
details.resurvey summary { cursor: pointer; font-size: var(--text-ui); color: hsl(var(--muted-foreground)); }
</style>`

const comparisonTemplate = `{{define "comparison"}}
<div class="cmp-body">
<div class="stat-row">
    <span>Resource types: <b>{{.New.Types}}</b> {{delta .Old.Types .New.Types}}</span>
    <span>Deposits: <b>{{.New.Deposits}}</b> {{delta .Old.Deposits .New.Deposits}}</span>
    <span>Systems explored: <b>{{.New.Explored}}</b> / {{.New.Systems}} {{delta .Old.Explored .New.Explored}}</span>
</div>
{{if not .HasChanges}}<p class="no-changes">No resource changes.</p>
{{else}}
{{if .NewTypes}}<h4>New resource types in the catalog</h4>
<ul class="diff-list">{{range .NewTypes}}<li class="diff-add">+ {{with itemURL .}}<a href="{{.}}">{{end}}{{.Name}}{{if itemURL .}}</a>{{end}} <span class="text-muted">({{.ID}})</span></li>{{end}}</ul>{{end}}
{{if .RemovedTypes}}<h4>Resource types removed from the catalog</h4>
<ul class="diff-list">{{range .RemovedTypes}}<li class="diff-del">&minus; {{.Name}} <span class="text-muted">({{.ID}})</span></li>{{end}}</ul>{{end}}
{{if .Discovered}}<h4>Newly discovered</h4>
<ul class="diff-list">{{range .Discovered}}<li class="diff-add">&#9733; {{with itemURL .ResourceType}}<a href="{{.}}">{{end}}{{.Name}}{{if itemURL .ResourceType}}</a>{{end}} <span class="text-muted">({{.ID}})</span> &mdash; first {{.Deposits}} {{plural .Deposits "deposit"}} found</li>{{end}}</ul>{{end}}
{{if .Lost}}<h4>No longer found anywhere</h4>
<ul class="diff-list">{{range .Lost}}<li class="diff-del">&minus; {{.Name}} <span class="text-muted">({{.ID}})</span></li>{{end}}</ul>{{end}}
{{if .NewPOIs}}<h4>New POIs with deposits <span class="diff-add">+{{len .NewPOIs}}</span>{{with .NewHiddenPOIs}} <span class="badge-hidden">{{.}} hidden</span>{{end}}</h4>
<table>
<thead><tr><th>System</th><th>POI</th><th>POI ID</th><th>Hidden</th><th>Deposits</th></tr></thead>
<tbody>{{range .NewPOIs}}
<tr><td><a href="../../systems/{{.SystemID}}/index.html">{{.SystemName}}</a></td><td>{{.Name}}</td><td><code>{{.ID}}</code></td><td>{{if .Hidden}}<span class="badge-hidden">hidden</span>{{else}}<span class="text-muted">&mdash;</span>{{end}}</td><td class="num">{{.Deposits}}</td></tr>{{end}}
</tbody></table>{{end}}
{{if .NewSystems}}<h4>Systems with their first deposits <span class="diff-add">+{{len .NewSystems}}</span></h4>
<ul class="diff-list">{{range .NewSystems}}<li class="diff-add">+ <a href="../../systems/{{.ID}}/index.html">{{.Name}}</a> <span class="text-muted">({{.ID}}, {{.Deposits}} {{plural .Deposits "deposit"}})</span></li>{{end}}</ul>{{end}}
{{if .NewDeposits}}<h4>New deposits <span class="diff-add">+{{.NewDepositCount}}</span></h4>
{{range .NewDeposits}}<h5>{{with itemURL .ResourceType}}<a href="{{.}}">{{end}}{{.Name}}{{if itemURL .ResourceType}}</a>{{end}} <span class="diff-add">+{{len .Rows}}</span></h5>
<table>
<thead><tr><th>System</th><th>POI</th><th>POI ID</th><th>Hidden</th><th>Station</th><th>Richness</th><th>Remaining</th><th title="Deposit capacity; only known when surveyed with get_poi">Capacity</th><th title="Mining power the deposit accepts now: floor(remaining / 20)">Power</th><th>Tick</th></tr></thead>
<tbody>{{range .Rows}}
<tr><td><a href="../../systems/{{.SystemID}}/index.html">{{.SystemName}}</a></td><td>{{.POIName}}</td><td><code>{{.POIID}}</code></td><td>{{if .Hidden}}<span class="badge-hidden">hidden</span>{{else}}<span class="text-muted">&mdash;</span>{{end}}</td><td>{{if .Station}}&#10003;{{else}}<span class="text-muted">&mdash;</span>{{end}}</td><td class="num">{{.Richness}}</td><td class="num">{{.Remaining}}</td><td class="num">{{if .MaxRemaining}}{{.MaxRemaining}}{{else}}<span class="text-muted">&mdash;</span>{{end}}</td><td class="num">{{.SupportedPower}}</td><td class="num">{{tick .LastTick}}</td></tr>{{end}}
</tbody></table>{{end}}{{end}}
{{if .RemovedDeposits}}<h4>Deposits no longer listed <span class="diff-del">&minus;{{len .RemovedDeposits}}</span></h4>
<table>
<thead><tr><th>Resource</th><th>System</th><th>POI</th><th>POI ID</th><th>Richness</th><th>Remaining</th></tr></thead>
<tbody>{{range .RemovedDeposits}}
<tr class="diff-del"><td>{{.ResourceName}}</td><td><a href="../../systems/{{.SystemID}}/index.html">{{.SystemName}}</a></td><td>{{.POIName}}</td><td><code>{{.POIID}}</code></td><td class="num">{{.Richness}}</td><td class="num">{{.Remaining}}</td></tr>{{end}}
</tbody></table>{{end}}
{{if .Changed}}<h4>Re-surveyed deposits <span class="diff-mod">~{{len .Changed}}</span></h4>
<details class="resurvey"><summary>{{len .Changed}} known {{plural (len .Changed) "deposit"}} changed richness, remaining amount, or hidden status</summary>
<table>
<thead><tr><th>Resource</th><th>System</th><th>POI</th><th>Richness</th><th>Remaining</th><th title="Mining power the deposit accepts: floor(remaining / 20)">Power</th><th>Hidden</th></tr></thead>
<tbody>{{range .Changed}}
<tr><td>{{.ResourceName}}</td><td><a href="../../systems/{{.New.SystemID}}/index.html">{{.New.SystemName}}</a></td><td>{{.New.POIName}}</td>
<td class="num">{{if ne .Old.Richness .New.Richness}}<span class="diff-del">{{.Old.Richness}}</span> &rarr; <span class="diff-add">{{.New.Richness}}</span>{{else}}{{.New.Richness}}{{end}}</td>
<td class="num">{{if ne .Old.Remaining .New.Remaining}}<span class="diff-del">{{.Old.Remaining}}</span> &rarr; <span class="diff-add">{{.New.Remaining}}</span>{{else}}{{.New.Remaining}}{{end}}</td>
<td class="num">{{if ne .Old.SupportedPower .New.SupportedPower}}<span class="diff-del">{{.Old.SupportedPower}}</span> &rarr; <span class="diff-add">{{.New.SupportedPower}}</span>{{else}}{{.New.SupportedPower}}{{end}}</td>
<td>{{if ne .Old.Hidden .New.Hidden}}<span class="diff-del">{{.Old.Hidden}}</span> &rarr; <span class="diff-add">{{.New.Hidden}}</span>{{else}}{{if .New.Hidden}}hidden{{else}}<span class="text-muted">&mdash;</span>{{end}}{{end}}</td></tr>{{end}}
</tbody></table></details>{{end}}
{{end}}
</div>
{{end}}`

const dayTemplate = comparisonTemplate + `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Resource Survey Changes &mdash; {{formatDate .Snapshot.Date}}</title>
<link rel="stylesheet" href="../../smui.css">
` + pageStyle + `
</head>
<body>
{{.Header}}
<main class="container page-content">
<div class="diff-nav">
    <span>{{if .PrevDate}}<a href="{{.PrevDate}}.html">&larr; {{.PrevDate}}</a>{{else}}&nbsp;{{end}}</span>
    <a href="index.html">All Reports</a>
    <span>{{if .NextDate}}<a href="{{.NextDate}}.html">{{.NextDate}} &rarr;</a>{{else}}&nbsp;{{end}}</span>
</div>
<h2>Resource Survey Changes &mdash; {{formatDate .Snapshot.Date}}</h2>
<div class="diff-summary">
Snapshot of the <a href="../index.html">Resources</a> page taken {{formatDate .Snapshot.Date}}{{with .Snapshot.ServerVersion}} on server v{{.}}{{end}}:
{{.Snapshot.Summary.Types}} resource types, {{.Snapshot.Summary.Deposits}} deposits, {{.Snapshot.Summary.Explored}} of {{.Snapshot.Summary.Systems}} systems explored.
{{if .IsBaseline}}<br>This snapshot is the <span class="baseline-tag">baseline</span> for content patch v{{.ContentVersion}}{{with .BaselineReason}} &mdash; {{.}}{{end}}.{{end}}
</div>

<div class="cmp-section" id="since-regen">
<div class="cmp-header">
<h3>Since the last regen{{with .VsPrevious}} &mdash; vs {{formatDate .OldDate}}{{end}}</h3>
{{if .VsPrevious}}<p>What the survey agents turned up between the previous KB regeneration and this one.{{if and .VsPrevious.OldVersion (ne .VsPrevious.OldVersion .Snapshot.ServerVersion)}} The server moved from v{{.VsPrevious.OldVersion}} to v{{.Snapshot.ServerVersion}} in between.{{end}}</p>{{end}}
</div>
{{if .VsPrevious}}{{template "comparison" .VsPrevious}}{{else}}<div class="cmp-body"><p class="no-changes">First snapshot &mdash; nothing to compare against yet.</p></div>{{end}}
</div>

<div class="cmp-section" id="since-update">
<div class="cmp-header">
<h3>Since the last server content update &mdash; patch v{{.ContentVersion}}{{with .ContentReleaseDate}} ({{formatDate .}}){{end}}{{if .IsBaseline}}, this snapshot is the baseline{{else}}, baseline snapshot <a href="{{.BaselineDate}}.html">{{formatDate .BaselineDate}}</a>{{end}}</h3>
<p>{{if .IsBaseline}}Later reports measure their discoveries against this snapshot until the next patch that changes resource content.{{else}}Everything found since the Resources page was snapshotted after patch v{{.ContentVersion}} landed.{{end}}{{with .CatalogDiffURL}} Catalog changes from that patch: <a href="{{.}}">game data diff</a>.{{end}}</p>
{{if .ContentNotes}}<ul class="diff-list">{{range .ContentNotes}}<li class="text-muted">&#8226; {{.}}</li>{{end}}</ul>{{end}}
</div>
{{if .VsBaseline}}{{template "comparison" .VsBaseline}}{{else}}<div class="cmp-body"><p class="no-changes">This snapshot is the baseline.</p></div>{{end}}
</div>
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
</html>`

const indexTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Resource Survey Change Log</title>
<link rel="stylesheet" href="../../smui.css">
` + pageStyle + `
</head>
<body>
{{.Header}}
<main class="container page-content">
<h2>Resource Survey Change Log</h2>
<p class="text-muted mt-1">Each KB regeneration snapshots the <a href="../index.html">Resources</a> page. Reports show what the survey agents found since the previous regen, and since the <span class="baseline-tag">baseline</span> snapshot taken after the last server content update. Catalog changes (items, ships, recipes) are tracked separately in the <a href="../../diffs/index.html">game data change log</a>.</p>
<table class="mt-2">
<thead><tr><th>Date</th><th>Server</th><th>Since last regen</th><th>Since server update</th></tr></thead>
<tbody>
{{range .Entries}}<tr><td><a href="{{.Date}}.html">{{.Date}}</a></td><td>{{with .ServerVersion}}v{{.}}{{else}}&mdash;{{end}}{{if .IsBaseline}} <span class="baseline-tag">baseline for v{{.ContentVersion}}</span>{{end}}</td><td>{{.VsPrevious}}</td><td>{{if .IsBaseline}}<span class="text-muted">&mdash;</span>{{else}}{{.VsBaseline}}{{end}}</td></tr>
{{end}}</tbody>
</table>
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
</html>`
