package gamediff

import (
	"bytes"
	"fmt"
	htmltpl "html/template"
	"time"
)

// DayReport holds all data needed to render a single day's diff page.
type DayReport struct {
	Date     time.Time
	PrevDate string // "YYYY-MM-DD" or "" if none
	NextDate string // "YYYY-MM-DD" or "" if none
	Catalogs []CatalogDiff
}

// TotalAdditions returns the sum of additions across all catalogs.
func (d DayReport) TotalAdditions() int {
	n := 0
	for _, c := range d.Catalogs {
		n += len(c.Additions)
	}
	return n
}

// TotalDeletions returns the sum of deletions across all catalogs.
func (d DayReport) TotalDeletions() int {
	n := 0
	for _, c := range d.Catalogs {
		n += len(c.Deletions)
	}
	return n
}

// TotalModifications returns the count of modified items (unique IDs) across all catalogs.
func (d DayReport) TotalModifications() int {
	n := 0
	for _, c := range d.Catalogs {
		seen := make(map[string]bool)
		for _, ch := range c.Changes {
			seen[ch.ID] = true
		}
		n += len(seen)
	}
	return n
}

// CatalogsWithChanges returns the count of catalogs that have any diff.
func (d DayReport) CatalogsWithChanges() int {
	n := 0
	for _, c := range d.Catalogs {
		if c.HasChanges() {
			n++
		}
	}
	return n
}

// GroupedChanges returns changes for a catalog grouped by item ID.
func GroupedChanges(changes []Change) map[string][]Change {
	m := make(map[string][]Change)
	for _, c := range changes {
		m[c.ID] = append(m[c.ID], c)
	}
	return m
}

// UniqueModifiedCount returns the number of unique IDs in changes.
func UniqueModifiedCount(changes []Change) int {
	seen := make(map[string]bool)
	for _, c := range changes {
		seen[c.ID] = true
	}
	return len(seen)
}

// IndexEntry is one row on the diffs index page.
type IndexEntry struct {
	Date    string // "YYYY-MM-DD"
	Summary string // e.g. "19 additions, 33 deletions" or "No changes"
}

var templateFuncs = htmltpl.FuncMap{
	"groupChanges":   GroupedChanges,
	"uniqueModCount": UniqueModifiedCount,
	"formatDate":     func(d time.Time) string { return d.Format("January 2, 2006") },
	"dateStr":        func(d time.Time) string { return d.Format("2006-01-02") },
	"add":            func(a, b int) int { return a + b },
}

// RenderDayReport renders a single day's diff report page as HTML.
func RenderDayReport(day DayReport) (string, error) {
	t, err := htmltpl.New("day").Funcs(templateFuncs).Parse(dayTemplate)
	if err != nil {
		return "", fmt.Errorf("parse day template: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, day); err != nil {
		return "", fmt.Errorf("execute day template: %w", err)
	}
	return buf.String(), nil
}

// RenderIndex renders the diffs index page listing all report days.
func RenderIndex(days []IndexEntry) (string, error) {
	t, err := htmltpl.New("index").Parse(indexTemplate)
	if err != nil {
		return "", fmt.Errorf("parse index template: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, days); err != nil {
		return "", fmt.Errorf("execute index template: %w", err)
	}
	return buf.String(), nil
}

const dayTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Game Data Changes — {{formatDate .Date}}</title>
<link rel="stylesheet" href="../smui.css">
<style>
.diff-nav { display: flex; justify-content: space-between; padding: 1rem 0; font-size: var(--text-ui); }
.diff-nav a { color: hsl(var(--primary)); }
.diff-summary { padding: 1rem 0; color: hsl(var(--muted-foreground)); font-size: var(--text-ui); }
.catalog-section { border: 1px solid hsl(var(--border)); margin: 1.5rem 0; }
.catalog-header { display: flex; justify-content: space-between; align-items: center; padding: 0.75rem 1rem; background: hsl(var(--card)); border-bottom: 1px solid hsl(var(--border)); }
.catalog-header h3 { font-size: var(--text-ui); font-weight: 600; margin: 0; }
.catalog-counts { display: flex; gap: 1rem; font-size: var(--text-label); letter-spacing: 0.5px; }
.catalog-body { padding: 1rem; }
.catalog-body h4 { font-size: var(--text-label); text-transform: uppercase; letter-spacing: 1.5px; color: hsl(var(--muted-foreground)); margin: 1rem 0 0.5rem; }
.catalog-body h4:first-child { margin-top: 0; }
.diff-add { color: hsl(var(--smui-green)); }
.diff-del { color: hsl(var(--smui-red)); }
.diff-mod { color: hsl(var(--smui-yellow)); }
.diff-list { list-style: none; padding: 0; }
.diff-list li { padding: 0.2rem 0; font-size: var(--text-ui); }
.diff-field { padding-left: 1.5rem; font-size: var(--text-ui); margin-bottom: 0.5rem; }
.diff-field-name { color: hsl(var(--muted-foreground)); }
.diff-item-id { font-weight: 600; margin-top: 0.5rem; }
.no-changes { color: hsl(var(--muted-foreground)); font-style: italic; }
</style>
</head>
<body>
<header class="site-header">
    <h1>Spacemolt KB</h1>
    <nav>
        <a href="../systems/">Systems</a>
        <a href="../items/">Items</a>
        <a href="../recipes/">Recipes</a>
        <a href="../skills/">Skills</a>
        <a href="../ships/">Ships</a>
    </nav>
</header>
<main class="container">
<div class="diff-nav">
    <span>{{if .PrevDate}}<a href="{{.PrevDate}}.html">&larr; {{.PrevDate}}</a>{{else}}&nbsp;{{end}}</span>
    <a href="index.html">All Reports</a>
    <span>{{if .NextDate}}<a href="{{.NextDate}}.html">{{.NextDate}} &rarr;</a>{{else}}&nbsp;{{end}}</span>
</div>
<h2>Game Data Changes &mdash; {{formatDate .Date}}</h2>
<div class="diff-summary">
{{- $adds := .TotalAdditions}}{{$dels := .TotalDeletions}}{{$mods := .TotalModifications}}{{$cats := .CatalogsWithChanges -}}
{{if eq (add $adds (add $dels $mods)) 0}}No changes detected across any catalog.
{{else}}{{$adds}} addition{{if ne $adds 1}}s{{end}}, {{$dels}} deletion{{if ne $dels 1}}s{{end}}, {{$mods}} modification{{if ne $mods 1}}s{{end}} across {{$cats}} catalog{{if ne $cats 1}}s{{end}}.
{{end}}</div>
{{range .Catalogs}}
<div class="catalog-section">
<div class="catalog-header">
    <h3>{{.Name}}</h3>
    <div class="catalog-counts">
        {{if .HasChanges}}
        <span class="diff-add">+{{len .Additions}}</span>
        <span class="diff-del">&minus;{{len .Deletions}}</span>
        <span class="diff-mod">~{{uniqueModCount .Changes}}</span>
        {{else}}
        <span class="no-changes">unchanged</span>
        {{end}}
    </div>
</div>
<div class="catalog-body">
{{if not .HasChanges}}<p class="no-changes">No changes</p>
{{else}}
{{if .Additions}}<h4>Additions</h4>
<ul class="diff-list">{{range .Additions}}<li class="diff-add">+ {{.Name}} <span class="text-muted">({{.ID}})</span></li>{{end}}</ul>{{end}}
{{if .Deletions}}<h4>Deletions</h4>
<ul class="diff-list">{{range .Deletions}}<li class="diff-del">&minus; {{.Name}} <span class="text-muted">({{.ID}})</span></li>{{end}}</ul>{{end}}
{{if .Changes}}<h4>Modified</h4>
{{range $id, $changes := groupChanges .Changes}}<div class="diff-item-id">{{$id}}</div>
{{range $changes}}<div class="diff-field"><span class="diff-field-name">{{.Field}}:</span> <span class="diff-del">{{.OldVal}}</span> &rarr; <span class="diff-add">{{.NewVal}}</span></div>{{end}}
{{end}}{{end}}
{{end}}
</div>
</div>
{{end}}
</main>
<script>document.documentElement.classList.add('dark');</script>
</body>
</html>`

const indexTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Game Data Change Log</title>
<link rel="stylesheet" href="../smui.css">
</head>
<body>
<header class="site-header">
    <h1>Spacemolt KB</h1>
    <nav>
        <a href="../systems/">Systems</a>
        <a href="../items/">Items</a>
        <a href="../recipes/">Recipes</a>
        <a href="../skills/">Skills</a>
        <a href="../ships/">Ships</a>
    </nav>
</header>
<main class="container">
<div class="page-content">
<h2>Game Data Change Log</h2>
<p class="text-muted mt-1">Daily diffs of catalog data (recipes, items, ships, skills, map).</p>
<table class="mt-2">
<thead><tr><th>Date</th><th>Changes</th></tr></thead>
<tbody>
{{range .}}<tr><td><a href="{{.Date}}.html">{{.Date}}</a></td><td>{{.Summary}}</td></tr>
{{end}}
</tbody>
</table>
</div>
</main>
<script>document.documentElement.classList.add('dark');</script>
</body>
</html>`
