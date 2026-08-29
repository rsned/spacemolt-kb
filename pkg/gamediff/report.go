package gamediff

import (
	"bytes"
	"encoding/json"
	"fmt"
	htmltpl "html/template"
	"slices"
	"strings"
	"time"
)

// DayReport holds all data needed to render a single day's diff page.
type DayReport struct {
	Date        time.Time
	CompareDate string // "YYYY-MM-DD" date of the snapshot being compared against
	PrevDate    string // "YYYY-MM-DD" or "" if none
	NextDate    string // "YYYY-MM-DD" or "" if none
	Catalogs    []CatalogDiff
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

// renderFieldDiff returns HTML for a single field change. For complex values
// (arrays of objects or objects) it produces a two-column comparison table.
// For simple scalars it falls back to the inline old → new format.
func renderFieldDiff(field, oldVal, newVal string) htmltpl.HTML {
	// Try to detect complex JSON values (arrays or objects).
	oldVal = strings.TrimSpace(oldVal)
	newVal = strings.TrimSpace(newVal)

	if isJSONArray(oldVal) || isJSONArray(newVal) {
		if html, ok := renderScalarArrayDiff(field, oldVal, newVal); ok {
			return html
		}
		if html, ok := renderArrayDiff(field, oldVal, newVal); ok {
			return html
		}
	}
	if isJSONObject(oldVal) || isJSONObject(newVal) {
		if html, ok := renderObjectDiff(field, oldVal, newVal); ok {
			return html
		}
	}

	// Simple scalar: inline format.
	return htmltpl.HTML(fmt.Sprintf(
		`<div class="diff-field"><span class="diff-field-name">%s:</span> <span class="diff-del">%s</span> &rarr; <span class="diff-add">%s</span></div>`,
		htmltpl.HTMLEscapeString(field),
		htmltpl.HTMLEscapeString(oldVal),
		htmltpl.HTMLEscapeString(newVal),
	))
}

func isJSONArray(s string) bool  { return strings.HasPrefix(s, "[") }
func isJSONObject(s string) bool { return strings.HasPrefix(s, "{") }

// renderScalarArrayDiff renders two JSON arrays of scalars (strings, numbers)
// as a side-by-side table, matching elements by value and sorting alphabetically.
func renderScalarArrayDiff(field, oldVal, newVal string) (htmltpl.HTML, bool) {
	var oldArr, newArr []any
	if oldVal != "null" && oldVal != "" {
		if err := json.Unmarshal([]byte(oldVal), &oldArr); err != nil {
			return "", false
		}
	}
	if newVal != "null" && newVal != "" {
		if err := json.Unmarshal([]byte(newVal), &newArr); err != nil {
			return "", false
		}
	}

	// Only handle scalar arrays — bail if any element is an object or array.
	for _, v := range append(oldArr, newArr...) {
		switch v.(type) {
		case map[string]any, []any:
			return "", false
		}
	}

	oldSet := make(map[string]int)
	for _, v := range oldArr {
		oldSet[formatValue(v)]++
	}
	newSet := make(map[string]int)
	for _, v := range newArr {
		newSet[formatValue(v)]++
	}

	allVals := make(map[string]bool)
	for k := range oldSet {
		allVals[k] = true
	}
	for k := range newSet {
		allVals[k] = true
	}
	sorted := make([]string, 0, len(allVals))
	for k := range allVals {
		sorted = append(sorted, k)
	}
	slices.Sort(sorted)

	var b strings.Builder
	b.WriteString(fmt.Sprintf(
		`<div class="diff-field"><span class="diff-field-name">%s:</span><table class="diff-compare"><thead><tr><th>Previous</th><th>Current</th></tr></thead><tbody>`,
		htmltpl.HTMLEscapeString(field),
	))

	for _, val := range sorted {
		oc := oldSet[val]
		nc := newSet[val]
		esc := htmltpl.HTMLEscapeString(val)

		// Matched pairs (unchanged).
		matched := min(oc, nc)
		for range matched {
			b.WriteString(fmt.Sprintf(`<tr><td class="diff-unchanged">%s</td><td class="diff-unchanged">%s</td></tr>`, esc, esc))
		}
		// Extra old copies (removed).
		for range oc - matched {
			b.WriteString(fmt.Sprintf(`<tr><td class="diff-del">%s</td><td class="diff-empty"></td></tr>`, esc))
		}
		// Extra new copies (added).
		for range nc - matched {
			b.WriteString(fmt.Sprintf(`<tr><td class="diff-empty"></td><td class="diff-add">%s</td></tr>`, esc))
		}
	}

	b.WriteString(`</tbody></table></div>`)
	return htmltpl.HTML(b.String()), true
}

// renderArrayDiff renders two JSON arrays of objects as a side-by-side table.
// Elements are matched by the first string field found (e.g. "type", "name").
func renderArrayDiff(field, oldVal, newVal string) (htmltpl.HTML, bool) {
	var oldArr, newArr []map[string]any
	if oldVal == "null" || oldVal == "" {
		oldArr = nil
	} else if err := json.Unmarshal([]byte(oldVal), &oldArr); err != nil {
		return "", false
	}
	if newVal == "null" || newVal == "" {
		newArr = nil
	} else if err := json.Unmarshal([]byte(newVal), &newArr); err != nil {
		return "", false
	}

	// Find the best key field to match on.
	keyField := findKeyField(oldArr, newArr)

	// Index both arrays by key.
	oldByKey := indexByField(oldArr, keyField)
	newByKey := indexByField(newArr, keyField)

	// Collect all keys, sorted.
	allKeys := make(map[string]bool)
	for k := range oldByKey {
		allKeys[k] = true
	}
	for k := range newByKey {
		allKeys[k] = true
	}
	sorted := make([]string, 0, len(allKeys))
	for k := range allKeys {
		sorted = append(sorted, k)
	}
	slices.Sort(sorted)

	var b strings.Builder
	b.WriteString(fmt.Sprintf(
		`<div class="diff-field"><span class="diff-field-name">%s:</span><table class="diff-compare"><thead><tr><th>Previous</th><th>Current</th></tr></thead><tbody>`,
		htmltpl.HTMLEscapeString(field),
	))

	for _, key := range sorted {
		oldObj := oldByKey[key]
		newObj := newByKey[key]
		oldText := formatObject(oldObj)
		newText := formatObject(newObj)

		if oldObj == nil {
			// Added
			b.WriteString(fmt.Sprintf(`<tr><td class="diff-empty"></td><td class="diff-add">%s</td></tr>`, htmltpl.HTMLEscapeString(newText)))
		} else if newObj == nil {
			// Removed
			b.WriteString(fmt.Sprintf(`<tr><td class="diff-del">%s</td><td class="diff-empty"></td></tr>`, htmltpl.HTMLEscapeString(oldText)))
		} else if oldText == newText {
			// Unchanged
			b.WriteString(fmt.Sprintf(`<tr><td class="diff-unchanged">%s</td><td class="diff-unchanged">%s</td></tr>`, htmltpl.HTMLEscapeString(oldText), htmltpl.HTMLEscapeString(newText)))
		} else {
			// Modified
			b.WriteString(fmt.Sprintf(`<tr><td class="diff-del">%s</td><td class="diff-add">%s</td></tr>`, htmltpl.HTMLEscapeString(oldText), htmltpl.HTMLEscapeString(newText)))
		}
	}

	b.WriteString(`</tbody></table></div>`)
	return htmltpl.HTML(b.String()), true
}

// renderObjectDiff renders two JSON objects as a side-by-side table keyed by field name.
func renderObjectDiff(field, oldVal, newVal string) (htmltpl.HTML, bool) {
	var oldObj, newObj map[string]any
	if oldVal == "null" || oldVal == "" {
		oldObj = nil
	} else if err := json.Unmarshal([]byte(oldVal), &oldObj); err != nil {
		return "", false
	}
	if newVal == "null" || newVal == "" {
		newObj = nil
	} else if err := json.Unmarshal([]byte(newVal), &newObj); err != nil {
		return "", false
	}

	allKeys := make(map[string]bool)
	for k := range oldObj {
		allKeys[k] = true
	}
	for k := range newObj {
		allKeys[k] = true
	}
	sorted := make([]string, 0, len(allKeys))
	for k := range allKeys {
		sorted = append(sorted, k)
	}
	slices.Sort(sorted)

	var b strings.Builder
	b.WriteString(fmt.Sprintf(
		`<div class="diff-field"><span class="diff-field-name">%s:</span><table class="diff-compare"><thead><tr><th>Previous</th><th>Current</th></tr></thead><tbody>`,
		htmltpl.HTMLEscapeString(field),
	))

	for _, key := range sorted {
		oldV, oldOK := oldObj[key]
		newV, newOK := newObj[key]
		oldText := formatValue(oldV)
		newText := formatValue(newV)

		if !oldOK {
			b.WriteString(fmt.Sprintf(`<tr><td class="diff-empty"></td><td class="diff-add">%s: %s</td></tr>`, htmltpl.HTMLEscapeString(key), htmltpl.HTMLEscapeString(newText)))
		} else if !newOK {
			b.WriteString(fmt.Sprintf(`<tr><td class="diff-del">%s: %s</td><td class="diff-empty"></td></tr>`, htmltpl.HTMLEscapeString(key), htmltpl.HTMLEscapeString(oldText)))
		} else if oldText == newText {
			b.WriteString(fmt.Sprintf(`<tr><td class="diff-unchanged">%s: %s</td><td class="diff-unchanged">%s: %s</td></tr>`, htmltpl.HTMLEscapeString(key), htmltpl.HTMLEscapeString(oldText), htmltpl.HTMLEscapeString(key), htmltpl.HTMLEscapeString(newText)))
		} else {
			b.WriteString(fmt.Sprintf(`<tr><td class="diff-del">%s: %s</td><td class="diff-add">%s: %s</td></tr>`, htmltpl.HTMLEscapeString(key), htmltpl.HTMLEscapeString(oldText), htmltpl.HTMLEscapeString(key), htmltpl.HTMLEscapeString(newText)))
		}
	}

	b.WriteString(`</tbody></table></div>`)
	return htmltpl.HTML(b.String()), true
}

// findKeyField picks the best string field to use as a match key across two arrays of objects.
func findKeyField(a, b []map[string]any) string {
	candidates := []string{"type", "name", "id", "key"}
	all := append(a, b...)
	for _, c := range candidates {
		found := true
		for _, obj := range all {
			if _, ok := obj[c].(string); !ok {
				found = false
				break
			}
		}
		if found && len(all) > 0 {
			return c
		}
	}
	// Fallback: use first string field alphabetically.
	for _, obj := range all {
		keys := make([]string, 0, len(obj))
		for k := range obj {
			if _, ok := obj[k].(string); ok {
				keys = append(keys, k)
			}
		}
		slices.Sort(keys)
		if len(keys) > 0 {
			return keys[0]
		}
	}
	return ""
}

func indexByField(arr []map[string]any, field string) map[string]map[string]any {
	m := make(map[string]map[string]any, len(arr))
	for i, obj := range arr {
		key, _ := obj[field].(string)
		if key == "" {
			key = fmt.Sprintf("_item_%d", i)
		}
		m[key] = obj
	}
	return m
}

// formatObject renders a map as "key: val, key: val" with sorted keys.
func formatObject(obj map[string]any) string {
	if obj == nil {
		return ""
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s: %s", k, formatValue(obj[k])))
	}
	return strings.Join(parts, ", ")
}

func formatValue(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case bool:
		return fmt.Sprintf("%t", val)
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

var templateFuncs = htmltpl.FuncMap{
	"groupChanges":   GroupedChanges,
	"uniqueModCount": UniqueModifiedCount,
	"formatDate":     func(d time.Time) string { return d.Format("January 2, 2006") },
	"dateStr":        func(d time.Time) string { return d.Format("2006-01-02") },
	"add":            func(a, b int) int { return a + b },
	"slugify":        func(s string) string { return strings.ToLower(strings.ReplaceAll(s, " ", "-")) },
	"renderFieldDiff": renderFieldDiff,
	"formatDateStr": func(s string) string {
		if t, err := time.Parse("2006-01-02", s); err == nil {
			return t.Format("January 2, 2006")
		}
		return s
	},
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
<title>Game Data Changes — {{formatDate .Date}}{{if .CompareDate}} vs {{formatDateStr .CompareDate}}{{end}}</title>
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
.diff-list a, .diff-item-id a { color: inherit; text-decoration: underline; text-underline-offset: 2px; }
.diff-list a:hover, .diff-item-id a:hover { color: hsl(var(--primary)); }
.diff-field { padding-left: 1.5rem; font-size: var(--text-ui); margin-bottom: 0.5rem; }
.diff-field-name { color: hsl(var(--muted-foreground)); }
.diff-item-id { font-weight: 600; margin-top: 0.5rem; }
.no-changes { color: hsl(var(--muted-foreground)); font-style: italic; }
.diff-compare { width: 100%; margin: 0.25rem 0 0.5rem 1.5rem; border-collapse: collapse; font-size: var(--text-ui); table-layout: fixed; }
.diff-compare th { text-align: left; padding: 0.25rem 0.5rem; color: hsl(var(--muted-foreground)); font-weight: 600; font-size: var(--text-label); border-bottom: 1px solid hsl(var(--border)); }
.diff-compare td { padding: 0.2rem 0.5rem; vertical-align: top; border-bottom: 1px solid hsl(var(--border) / 0.3); }
.diff-compare .diff-unchanged { color: hsl(var(--foreground)); }
.diff-compare .diff-empty { }
.diff-toc { padding: 0.5rem 0; font-size: var(--text-ui); list-style: none; margin: 0; }
.diff-toc li { padding: 0.15rem 0; }
.diff-toc a { color: hsl(var(--primary)); }
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
<h2>Game Data Changes &mdash; {{formatDate .Date}}{{if .CompareDate}} vs {{formatDateStr .CompareDate}}{{end}}</h2>
<div class="diff-summary">
{{- $adds := .TotalAdditions}}{{$dels := .TotalDeletions}}{{$mods := .TotalModifications}}{{$cats := .CatalogsWithChanges -}}
{{if eq (add $adds (add $dels $mods)) 0}}No changes detected across any catalog.
{{else}}{{$adds}} addition{{if ne $adds 1}}s{{end}}, {{$dels}} deletion{{if ne $dels 1}}s{{end}}, {{$mods}} modification{{if ne $mods 1}}s{{end}} across {{$cats}} catalog{{if ne $cats 1}}s{{end}}.
{{end}}</div>
<ul class="diff-toc">{{range .Catalogs}}
<li><a href="#{{slugify .Name}}">{{.Name}}</a>{{if .HasChanges}} <span class="diff-add">+{{len .Additions}}</span> <span class="diff-del">&minus;{{len .Deletions}}</span> <span class="diff-mod">~{{uniqueModCount .Changes}}</span>{{end}}</li>{{end}}
</ul>
{{range .Catalogs}}
<div class="catalog-section" id="{{slugify .Name}}">
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
<ul class="diff-list">{{range .Additions}}<li class="diff-add">+ {{if .URL}}<a href="{{.URL}}">{{.Name}}</a>{{else}}{{.Name}}{{end}} <span class="text-muted">({{.ID}})</span></li>{{end}}</ul>{{end}}
{{if .Deletions}}<h4>Deletions</h4>
<ul class="diff-list">{{range .Deletions}}<li class="diff-del">&minus; {{if .URL}}<a href="{{.URL}}">{{.Name}}</a>{{else}}{{.Name}}{{end}} <span class="text-muted">({{.ID}})</span></li>{{end}}</ul>{{end}}
{{if .Changes}}<h4>Modified</h4>
{{range $id, $changes := groupChanges .Changes}}<div class="diff-item-id">{{with (index $changes 0).URL}}<a href="{{.}}">{{$id}}</a>{{else}}{{$id}}{{end}}</div>
{{range $changes}}{{renderFieldDiff .Field .OldVal .NewVal}}{{end}}
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
<p class="text-muted">Survey-driven changes to the Resources page (newly discovered ores, new deposits, new POIs) are tracked in the <a href="../resources/changes/index.html">resource survey change log</a>.</p>
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
