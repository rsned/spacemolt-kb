# Daily Game Data Diff Reports — Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Automate daily diffing of the 5 static game catalogs and generate HTML report pages at `kb/diffs/`.

**Architecture:** A `pkg/gamediff` library handles JSON comparison (ported from `spacemolt/cmd/data/data-diff`). A `cmd/generate-diffs` CLI tool manages snapshot storage (dated directories with symlinks), runs diffs, and generates HTML using Go `html/template`. Output uses the existing `smui.css` dark theme.

**Tech Stack:** Go 1.24, `encoding/json`, `html/template`, `os` (symlinks), existing `smui.css`

---

## File Structure

```
kb/
├── cmd/generate-diffs/main.go      # CLI: snapshot management + HTML generation
├── pkg/gamediff/
│   ├── diff.go                     # JSON comparison logic
│   ├── diff_test.go                # Unit tests for diff logic
│   ├── report.go                   # HTML template rendering
│   └── report_test.go              # Tests for HTML output
├── data/snapshots/                  # Runtime: dated JSON dirs + symlinks (gitignored)
└── kb/diffs/                        # Runtime: generated HTML (gitignored)
```

---

### Task 1: Diff library — core types and catalog comparison

**Files:**
- Create: `pkg/gamediff/diff.go`
- Create: `pkg/gamediff/diff_test.go`

- [ ] **Step 1: Write failing tests for catalog diffing**

Create `pkg/gamediff/diff_test.go`:

```go
package gamediff

import (
	"testing"
)

func TestDiffCatalog_Additions(t *testing.T) {
	old := []byte(`{"items":[{"id":"a","name":"Alpha"}]}`)
	new := []byte(`{"items":[{"id":"a","name":"Alpha"},{"id":"b","name":"Beta"}]}`)

	result, err := DiffCatalog(old, new)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Additions) != 1 {
		t.Fatalf("want 1 addition, got %d", len(result.Additions))
	}
	if result.Additions[0].ID != "b" {
		t.Errorf("want addition id=b, got %s", result.Additions[0].ID)
	}
}

func TestDiffCatalog_Deletions(t *testing.T) {
	old := []byte(`{"items":[{"id":"a","name":"Alpha"},{"id":"b","name":"Beta"}]}`)
	new := []byte(`{"items":[{"id":"a","name":"Alpha"}]}`)

	result, err := DiffCatalog(old, new)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Deletions) != 1 {
		t.Fatalf("want 1 deletion, got %d", len(result.Deletions))
	}
	if result.Deletions[0].ID != "b" {
		t.Errorf("want deletion id=b, got %s", result.Deletions[0].ID)
	}
}

func TestDiffCatalog_Modifications(t *testing.T) {
	old := []byte(`{"items":[{"id":"a","name":"Alpha","value":10}]}`)
	new := []byte(`{"items":[{"id":"a","name":"Alpha","value":20}]}`)

	result, err := DiffCatalog(old, new)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Changes) != 1 {
		t.Fatalf("want 1 change, got %d", len(result.Changes))
	}
	if result.Changes[0].Field != "value" {
		t.Errorf("want field=value, got %s", result.Changes[0].Field)
	}
}

func TestDiffCatalog_NoChanges(t *testing.T) {
	data := []byte(`{"items":[{"id":"a","name":"Alpha"}]}`)

	result, err := DiffCatalog(data, data)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Additions) != 0 || len(result.Deletions) != 0 || len(result.Changes) != 0 {
		t.Error("expected no changes")
	}
}

func TestDiffMap_Additions(t *testing.T) {
	old := []byte(`{"systems":[{"system_id":"sol","name":"Sol"}]}`)
	new := []byte(`{"systems":[{"system_id":"sol","name":"Sol"},{"system_id":"nexus","name":"Nexus"}]}`)

	result, err := DiffMap(old, new)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Additions) != 1 {
		t.Fatalf("want 1 addition, got %d", len(result.Additions))
	}
	if result.Additions[0].ID != "nexus" {
		t.Errorf("want addition id=nexus, got %s", result.Additions[0].ID)
	}
}

func TestDiffMap_Modifications(t *testing.T) {
	old := []byte(`{"systems":[{"system_id":"sol","name":"Sol","security":80}]}`)
	new := []byte(`{"systems":[{"system_id":"sol","name":"Sol","security":100}]}`)

	result, err := DiffMap(old, new)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Changes) != 1 {
		t.Fatalf("want 1 change, got %d", len(result.Changes))
	}
	if result.Changes[0].Field != "security" {
		t.Errorf("want field=security, got %s", result.Changes[0].Field)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/gamediff/ -v`
Expected: FAIL — package does not exist yet.

- [ ] **Step 3: Implement diff.go**

Create `pkg/gamediff/diff.go`:

```go
// Package gamediff compares game API JSON snapshots and reports structural changes.
package gamediff

import (
	"encoding/json"
	"fmt"
	"slices"
)

// Entry is a named item that was added or removed.
type Entry struct {
	Name string
	ID   string
}

// Change is a field-level modification on a single item.
type Change struct {
	ID     string
	Field  string
	OldVal string // JSON-encoded
	NewVal string // JSON-encoded
}

// CatalogDiff holds the result of comparing two snapshots of a single catalog.
type CatalogDiff struct {
	Name      string   // display name, e.g. "Recipes"
	File      string   // filename, e.g. "catalog_recipes.json"
	Additions []Entry
	Deletions []Entry
	Changes   []Change
}

// HasChanges reports whether this catalog has any differences.
func (d *CatalogDiff) HasChanges() bool {
	return len(d.Additions) > 0 || len(d.Deletions) > 0 || len(d.Changes) > 0
}

// DiffCatalog compares two catalog JSON blobs (top-level {"items":[...]}).
func DiffCatalog(oldData, newData []byte) (*CatalogDiff, error) {
	oldItems, err := extractArray(oldData, "items", "id")
	if err != nil {
		return nil, fmt.Errorf("old: %w", err)
	}
	newItems, err := extractArray(newData, "items", "id")
	if err != nil {
		return nil, fmt.Errorf("new: %w", err)
	}
	return diffMaps(oldItems, newItems, "name"), nil
}

// DiffMap compares two map JSON blobs (top-level {"systems":[...]}).
func DiffMap(oldData, newData []byte) (*CatalogDiff, error) {
	oldSystems, err := extractArray(oldData, "systems", "system_id")
	if err != nil {
		return nil, fmt.Errorf("old: %w", err)
	}
	newSystems, err := extractArray(newData, "systems", "system_id")
	if err != nil {
		return nil, fmt.Errorf("new: %w", err)
	}
	return diffMaps(oldSystems, newSystems, "name"), nil
}

// extractArray parses JSON, pulls out the named top-level array, and indexes
// each element by the given key field.
func extractArray(data []byte, arrayField, keyField string) (map[string]map[string]any, error) {
	var top map[string]any
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, err
	}
	arr, ok := top[arrayField].([]any)
	if !ok {
		return nil, fmt.Errorf("field %q is not an array", arrayField)
	}
	result := make(map[string]map[string]any, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id, ok := m[keyField].(string)
		if !ok {
			continue
		}
		result[id] = m
	}
	return result, nil
}

// diffMaps compares two indexed maps and returns additions, deletions, and
// field-level changes. nameField is used for display names.
func diffMaps(oldMap, newMap map[string]map[string]any, nameField string) *CatalogDiff {
	result := &CatalogDiff{}

	for id, oldItem := range oldMap {
		newItem, exists := newMap[id]
		if !exists {
			result.Deletions = append(result.Deletions, Entry{
				ID:   id,
				Name: stringField(oldItem, nameField),
			})
			continue
		}
		// Field-level changes.
		for key, newVal := range newItem {
			oldVal, exists := oldItem[key]
			if !exists {
				continue
			}
			oldJSON := normalizeJSON(oldVal)
			newJSON := normalizeJSON(newVal)
			if oldJSON != newJSON {
				result.Changes = append(result.Changes, Change{
					ID:     id,
					Field:  key,
					OldVal: oldJSON,
					NewVal: newJSON,
				})
			}
		}
	}

	for id, newItem := range newMap {
		if _, exists := oldMap[id]; !exists {
			result.Additions = append(result.Additions, Entry{
				ID:   id,
				Name: stringField(newItem, nameField),
			})
		}
	}

	slices.SortFunc(result.Additions, func(a, b Entry) int {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	})
	slices.SortFunc(result.Deletions, func(a, b Entry) int {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	})

	return result
}

func stringField(m map[string]any, key string) string {
	if s, ok := m[key].(string); ok {
		return s
	}
	return "<unnamed>"
}

// normalizeJSON converts a value to a canonical JSON string for comparison.
func normalizeJSON(v any) string {
	normalized := normalizeValue(v)
	b, _ := json.Marshal(normalized)
	return string(b)
}

func normalizeValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		out := make(map[string]any, len(val))
		for _, k := range keys {
			out[k] = normalizeValue(val[k])
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = normalizeValue(item)
		}
		return out
	default:
		return v
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/gamediff/ -v`
Expected: all 6 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gamediff/diff.go pkg/gamediff/diff_test.go
git commit -m "feat(gamediff): add catalog and map JSON diff library"
```

---

### Task 2: HTML report generation

**Files:**
- Create: `pkg/gamediff/report.go`
- Create: `pkg/gamediff/report_test.go`

- [ ] **Step 1: Write failing test for report rendering**

Create `pkg/gamediff/report_test.go`:

```go
package gamediff

import (
	"strings"
	"testing"
	"time"
)

func TestRenderDayReport_ContainsDate(t *testing.T) {
	day := DayReport{
		Date:     time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC),
		PrevDate: "2026-03-11",
		NextDate: "",
		Catalogs: []CatalogDiff{
			{Name: "Recipes", File: "catalog_recipes.json"},
		},
	}
	html, err := RenderDayReport(day)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "March 12, 2026") {
		t.Error("report should contain formatted date")
	}
	if !strings.Contains(html, "2026-03-11") {
		t.Error("report should contain prev link")
	}
	if !strings.Contains(html, "No changes") {
		t.Error("empty catalog should show 'No changes'")
	}
}

func TestRenderDayReport_ShowsAdditions(t *testing.T) {
	day := DayReport{
		Date: time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC),
		Catalogs: []CatalogDiff{
			{
				Name:      "Items",
				Additions: []Entry{{Name: "Copper Ore", ID: "copper_ore"}},
			},
		},
	}
	html, err := RenderDayReport(day)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "Copper Ore") {
		t.Error("report should contain addition name")
	}
	if !strings.Contains(html, "+1") {
		t.Error("report should contain addition count")
	}
}

func TestRenderIndex_ListsDays(t *testing.T) {
	days := []IndexEntry{
		{Date: "2026-03-12", Summary: "19 additions, 33 deletions"},
		{Date: "2026-03-11", Summary: "No changes"},
	}
	html, err := RenderIndex(days)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, "2026-03-12") {
		t.Error("index should contain day link")
	}
	if !strings.Contains(html, "No changes") {
		t.Error("index should contain summary text")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./pkg/gamediff/ -v`
Expected: FAIL — `DayReport`, `RenderDayReport`, etc. undefined.

- [ ] **Step 3: Implement report.go**

Create `pkg/gamediff/report.go`. This file defines the `DayReport` and `IndexEntry` types plus the two HTML template rendering functions. The templates use `smui.css` via a relative path (`../smui.css`) and match the dark-theme site header pattern from `kb/index.html`.

```go
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

// RenderDayReport renders a single day's diff report page as HTML.
func RenderDayReport(day DayReport) (string, error) {
	t, err := htmltpl.New("day").Funcs(htmltpl.FuncMap{
		"groupChanges":       GroupedChanges,
		"uniqueModCount":     UniqueModifiedCount,
		"formatDate":         func(d time.Time) string { return d.Format("January 2, 2006") },
		"dateStr":            func(d time.Time) string { return d.Format("2006-01-02") },
	}).Parse(dayTemplate)
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
.diff-field { padding-left: 1.5rem; font-size: var(--text-ui); }
.diff-field-name { color: hsl(var(--muted-foreground)); }
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
{{range $id, $changes := groupChanges .Changes}}<div class="diff-field"><strong>{{$id}}</strong>
{{range $changes}}<div class="diff-field"><span class="diff-field-name">{{.Field}}:</span> <span class="diff-del">{{.OldVal}}</span> &rarr; <span class="diff-add">{{.NewVal}}</span></div>{{end}}
</div>{{end}}{{end}}
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
<style>
.diff-table { margin-top: 1.5rem; }
.diff-table td:first-child { white-space: nowrap; }
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
<div class="page-content">
<h2>Game Data Change Log</h2>
<p class="text-muted mt-1">Daily diffs of catalog data (recipes, items, ships, skills, map).</p>
<table class="diff-table">
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
```

Note: the template needs an `add` function for the zero-check. Add it to the FuncMap:

```go
"add": func(a, b int) int { return a + b },
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/gamediff/ -v`
Expected: all tests PASS (both diff_test.go and report_test.go).

- [ ] **Step 5: Commit**

```bash
git add pkg/gamediff/report.go pkg/gamediff/report_test.go
git commit -m "feat(gamediff): add HTML report rendering for day and index pages"
```

---

### Task 3: CLI tool — snapshot management and orchestration

**Files:**
- Create: `cmd/generate-diffs/main.go`

- [ ] **Step 1: Implement the CLI tool**

Create `cmd/generate-diffs/main.go`:

```go
// Command generate-diffs manages game data snapshots and generates HTML diff
// reports. It compares fresh JSON catalog files against the previous snapshot,
// writes an HTML report page, and rotates the snapshot symlinks.
//
// Usage:
//
//	generate-diffs --input /path/to/fresh/json
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/rsned/spacemolt-kb/pkg/gamediff"
)

// catalogs defines the 5 tracked catalog files in display order.
var catalogs = []struct {
	name string // display name
	file string // filename
	isMap bool  // true for get_map.json (uses system_id key)
}{
	{"Recipes", "catalog_recipes.json", false},
	{"Items", "catalog_items.json", false},
	{"Ships", "catalog_ships.json", false},
	{"Skills", "catalog_skills.json", false},
	{"Map", "get_map.json", true},
}

func main() {
	inputDir := flag.String("input", "", "directory containing fresh JSON catalog files")
	snapshotDir := flag.String("snapshots", "data/snapshots", "base directory for snapshot storage")
	outputDir := flag.String("output", "kb/diffs", "output directory for HTML reports")
	flag.Parse()

	if *inputDir == "" {
		fmt.Fprintln(os.Stderr, "Usage: generate-diffs --input <dir>")
		flag.PrintDefaults()
		os.Exit(1)
	}

	today := time.Now().Format("20060102")
	todayDash := time.Now().Format("2006-01-02")

	// 1. Create today's snapshot directory and copy files.
	todayDir := filepath.Join(*snapshotDir, today)
	if err := os.MkdirAll(todayDir, 0o755); err != nil {
		log.Fatalf("create snapshot dir: %v", err)
	}
	for _, cat := range catalogs {
		src := filepath.Join(*inputDir, cat.file)
		dst := filepath.Join(todayDir, cat.file)
		data, err := os.ReadFile(src)
		if err != nil {
			log.Printf("warning: %s not found in input, skipping", cat.file)
			continue
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			log.Fatalf("write snapshot %s: %v", cat.file, err)
		}
	}
	log.Printf("Snapshot saved to %s", todayDir)

	// 2. Check for previous snapshot.
	prevLink := filepath.Join(*snapshotDir, "previous")
	prevTarget, err := os.Readlink(prevLink)
	if err != nil {
		// First run — no previous snapshot. Set up symlinks and exit.
		log.Println("No previous snapshot found (first run). Creating latest symlink.")
		latestLink := filepath.Join(*snapshotDir, "latest")
		_ = os.Remove(latestLink)
		if err := os.Symlink(today, latestLink); err != nil {
			log.Fatalf("create latest symlink: %v", err)
		}
		log.Println("Run again after next scrape to generate first diff report.")
		return
	}

	prevDir := filepath.Join(*snapshotDir, filepath.Base(prevTarget))

	// 3. Diff each catalog.
	var diffs []gamediff.CatalogDiff
	for _, cat := range catalogs {
		oldPath := filepath.Join(prevDir, cat.file)
		newPath := filepath.Join(todayDir, cat.file)

		oldData, err := os.ReadFile(oldPath)
		if err != nil {
			log.Printf("warning: cannot read previous %s, skipping", cat.file)
			diffs = append(diffs, gamediff.CatalogDiff{Name: cat.name, File: cat.file})
			continue
		}
		newData, err := os.ReadFile(newPath)
		if err != nil {
			log.Printf("warning: cannot read new %s, skipping", cat.file)
			diffs = append(diffs, gamediff.CatalogDiff{Name: cat.name, File: cat.file})
			continue
		}

		var result *gamediff.CatalogDiff
		if cat.isMap {
			result, err = gamediff.DiffMap(oldData, newData)
		} else {
			result, err = gamediff.DiffCatalog(oldData, newData)
		}
		if err != nil {
			log.Printf("warning: diff %s failed: %v", cat.file, err)
			diffs = append(diffs, gamediff.CatalogDiff{Name: cat.name, File: cat.file})
			continue
		}
		result.Name = cat.name
		result.File = cat.file
		diffs = append(diffs, *result)
	}

	// 4. Build the day report.
	if err := os.MkdirAll(*outputDir, 0o755); err != nil {
		log.Fatalf("create output dir: %v", err)
	}

	// Scan existing report files to find prev/next dates.
	existingDates := scanReportDates(*outputDir)
	existingDates = append(existingDates, todayDash)
	slices.Sort(existingDates)
	existingDates = slices.Compact(existingDates)

	todayIdx := slices.Index(existingDates, todayDash)
	prevDate := ""
	nextDate := ""
	if todayIdx > 0 {
		prevDate = existingDates[todayIdx-1]
	}
	if todayIdx < len(existingDates)-1 {
		nextDate = existingDates[todayIdx+1]
	}

	day := gamediff.DayReport{
		Date:     time.Now(),
		PrevDate: prevDate,
		NextDate: nextDate,
		Catalogs: diffs,
	}

	html, err := gamediff.RenderDayReport(day)
	if err != nil {
		log.Fatalf("render day report: %v", err)
	}
	dayFile := filepath.Join(*outputDir, todayDash+".html")
	if err := os.WriteFile(dayFile, []byte(html), 0o644); err != nil {
		log.Fatalf("write day report: %v", err)
	}
	log.Printf("Report written to %s", dayFile)

	// 5. Update prev/next links on adjacent reports.
	if prevDate != "" {
		updateAdjacentReport(*outputDir, prevDate, existingDates)
	}
	if nextDate != "" {
		updateAdjacentReport(*outputDir, nextDate, existingDates)
	}

	// 6. Regenerate index.
	var entries []gamediff.IndexEntry
	for i := len(existingDates) - 1; i >= 0; i-- {
		d := existingDates[i]
		summary := summarizeReport(*outputDir, d)
		entries = append(entries, gamediff.IndexEntry{Date: d, Summary: summary})
	}
	idxHTML, err := gamediff.RenderIndex(entries)
	if err != nil {
		log.Fatalf("render index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(*outputDir, "index.html"), []byte(idxHTML), 0o644); err != nil {
		log.Fatalf("write index: %v", err)
	}
	log.Println("Index updated")

	// 7. Rotate symlinks: previous -> old latest, latest -> today.
	latestLink := filepath.Join(*snapshotDir, "latest")
	_ = os.Remove(prevLink)
	// Previous points to what latest was pointing to.
	oldLatest, err := os.Readlink(latestLink)
	if err == nil {
		if err := os.Symlink(filepath.Base(oldLatest), prevLink); err != nil {
			log.Fatalf("update previous symlink: %v", err)
		}
	}
	_ = os.Remove(latestLink)
	if err := os.Symlink(today, latestLink); err != nil {
		log.Fatalf("update latest symlink: %v", err)
	}
	log.Printf("Symlinks updated: previous -> %s, latest -> %s",
		filepath.Base(oldLatest), today)
}

// scanReportDates returns all YYYY-MM-DD dates that have report HTML files.
func scanReportDates(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var dates []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".html") && name != "index.html" {
			date := strings.TrimSuffix(name, ".html")
			// Validate YYYY-MM-DD format.
			if _, err := time.Parse("2006-01-02", date); err == nil {
				dates = append(dates, date)
			}
		}
	}
	return dates
}

// summarizeReport reads a report file and builds a one-line summary.
// For simplicity, we re-count from the filename rather than parsing HTML.
// This is called during index generation when we have the data in memory
// for today, but for older reports we produce a simple label.
func summarizeReport(dir, date string) string {
	// For the current run we could pass data directly, but keeping it simple:
	// just read the file and look for count markers. This is a fallback —
	// the index is regenerated every run, so older entries get their summary
	// from the HTML content.
	path := filepath.Join(dir, date+".html")
	data, err := os.ReadFile(path)
	if err != nil {
		return "Report unavailable"
	}
	content := string(data)
	if strings.Contains(content, "No changes detected") {
		return "No changes"
	}
	// Extract the summary line from the diff-summary div.
	start := strings.Index(content, `<div class="diff-summary">`)
	if start < 0 {
		return "Changes detected"
	}
	end := strings.Index(content[start:], `</div>`)
	if end < 0 {
		return "Changes detected"
	}
	summary := content[start+len(`<div class="diff-summary">`): start+end]
	summary = strings.TrimSpace(summary)
	// Strip any remaining HTML tags.
	for strings.Contains(summary, "<") {
		s := strings.Index(summary, "<")
		e := strings.Index(summary, ">")
		if e < 0 {
			break
		}
		summary = summary[:s] + summary[e+1:]
	}
	return strings.TrimSpace(summary)
}

// updateAdjacentReport re-renders an existing report with updated prev/next links.
// It reads the HTML, finds the nav links, and rewrites them. For simplicity,
// we regenerate from scratch only if we stored the diff data, but since we don't
// persist diff data, we do simple string replacement on the nav div.
func updateAdjacentReport(dir, date string, allDates []string) {
	idx := slices.Index(allDates, date)
	if idx < 0 {
		return
	}
	prev := ""
	next := ""
	if idx > 0 {
		prev = allDates[idx-1]
	}
	if idx < len(allDates)-1 {
		next = allDates[idx+1]
	}

	path := filepath.Join(dir, date+".html")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	content := string(data)

	// Find and replace the diff-nav div.
	navStart := strings.Index(content, `<div class="diff-nav">`)
	if navStart < 0 {
		return
	}
	navEnd := strings.Index(content[navStart:], `</div>`)
	if navEnd < 0 {
		return
	}
	navEnd += navStart + len(`</div>`)

	var newNav strings.Builder
	newNav.WriteString(`<div class="diff-nav">`)
	newNav.WriteString("\n    <span>")
	if prev != "" {
		fmt.Fprintf(&newNav, `<a href="%s.html">&larr; %s</a>`, prev, prev)
	} else {
		newNav.WriteString("&nbsp;")
	}
	newNav.WriteString("</span>\n    ")
	newNav.WriteString(`<a href="index.html">All Reports</a>`)
	newNav.WriteString("\n    <span>")
	if next != "" {
		fmt.Fprintf(&newNav, `<a href="%s.html">%s &rarr;</a>`, next, next)
	} else {
		newNav.WriteString("&nbsp;")
	}
	newNav.WriteString("</span>\n</div>")

	newContent := content[:navStart] + newNav.String() + content[navEnd:]
	_ = os.WriteFile(path, []byte(newContent), 0o644)
}
```

- [ ] **Step 2: Verify it builds**

Run: `go build ./cmd/generate-diffs/`
Expected: builds successfully.

- [ ] **Step 3: Commit**

```bash
git add cmd/generate-diffs/main.go
git commit -m "feat: add generate-diffs CLI for snapshot management and report generation"
```

---

### Task 4: Gitignore and integration test

**Files:**
- Modify: `.gitignore`

- [ ] **Step 1: Add gitignore entries for runtime data**

Append to `.gitignore`:

```
# Diff report runtime data
data/snapshots/
kb/diffs/
```

- [ ] **Step 2: Run full build and tests**

Run: `go build ./... && go test ./...`
Expected: all packages build, all tests pass.

- [ ] **Step 3: Manual smoke test**

Run with sample data to verify end-to-end:

```bash
# Create a fake "old" snapshot
mkdir -p data/snapshots/20260311
echo '{"items":[{"id":"a","name":"Alpha"}]}' > data/snapshots/20260311/catalog_recipes.json
echo '{"items":[{"id":"a","name":"Alpha"}]}' > data/snapshots/20260311/catalog_items.json
echo '{"items":[{"id":"a","name":"Alpha"}]}' > data/snapshots/20260311/catalog_ships.json
echo '{"items":[{"id":"a","name":"Alpha"}]}' > data/snapshots/20260311/catalog_skills.json
echo '{"systems":[{"system_id":"sol","name":"Sol"}]}' > data/snapshots/20260311/get_map.json
ln -sf 20260311 data/snapshots/latest
ln -sf 20260311 data/snapshots/previous

# Create a "new" input with changes
mkdir -p /tmp/fresh-scrape
echo '{"items":[{"id":"a","name":"Alpha"},{"id":"b","name":"Beta"}]}' > /tmp/fresh-scrape/catalog_recipes.json
echo '{"items":[{"id":"a","name":"Alpha"}]}' > /tmp/fresh-scrape/catalog_items.json
echo '{"items":[{"id":"a","name":"Alpha"}]}' > /tmp/fresh-scrape/catalog_ships.json
echo '{"items":[{"id":"a","name":"Alpha"}]}' > /tmp/fresh-scrape/catalog_skills.json
echo '{"systems":[{"system_id":"sol","name":"Sol"}]}' > /tmp/fresh-scrape/get_map.json

# Run the tool
go run ./cmd/generate-diffs --input /tmp/fresh-scrape
```

Expected: creates `kb/diffs/2026-03-12.html` and `kb/diffs/index.html`. Open in browser to verify styling.

- [ ] **Step 4: Clean up test data and commit**

```bash
rm -rf data/snapshots /tmp/fresh-scrape kb/diffs
git add .gitignore
git commit -m "chore: gitignore snapshot and diff report runtime data"
```

---

### Task 5: Lint and final verification

- [ ] **Step 1: Run linter**

Run: `golangci-lint run ./pkg/gamediff/... ./cmd/generate-diffs/...`
Expected: no new findings.

- [ ] **Step 2: Fix any lint issues found**

- [ ] **Step 3: Final commit if needed**
