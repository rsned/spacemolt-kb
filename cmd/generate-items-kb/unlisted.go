package main

import (
	"cmp"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	htmltpl "html/template"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Unlisted recipes: recipes a mission unlocks that the public catalog does not
// list. A catalog id lookup for them returns not_found, so their details can
// only be captured from the unlocking character's craft dry-run quote. The
// details live in data/recipes_unlisted.json; which mission unlocks each one
// comes from the knowledge DB's mission_completions.recipes_unlocked.

// UnlistedRecipe is one recipe on the Unlisted page.
type UnlistedRecipe struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Inputs     []UnlistedItem `json:"inputs"`
	Outputs    []UnlistedItem `json:"outputs"`
	TimePerRun float64        `json:"time_per_run"` // quoted, skill-adjusted
	Venue      string         `json:"venue"`
	Captured   string         `json:"captured"`
	CapturedBy string         `json:"captured_by"`
	Note       string         `json:"note"`

	UnlockedBy []MissionRef `json:"-"`
	CatalogURL string       `json:"-"` // set when the catalog lists it after all
	Missing    bool         `json:"-"` // unlocked in-game, details not captured yet
}

// UnlistedItem is a recipe input or output.
type UnlistedItem struct {
	ItemID   string `json:"item_id"`
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
	URL      string `json:"-"` // item page; empty when the item is not in the catalog
}

// MissionRef links to the mission page that unlocks a recipe.
type MissionRef struct {
	ID, Title, Type string
}

// URL is the mission page, relative to kb/recipes/.
func (m MissionRef) URL() string {
	if m.Type == "" {
		return ""
	}
	return fmt.Sprintf("../missions/%s/%s.html", m.Type, m.ID)
}

// loadUnlistedFile reads the hand-captured recipe details. A missing file is
// an empty list.
func loadUnlistedFile(path string) ([]*UnlistedRecipe, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var f struct {
		Recipes []*UnlistedRecipe `json:"recipes"`
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return f.Recipes, nil
}

// loadRecipeUnlocks maps recipe id → the missions whose completion unlocked
// it, from mission_completions. Older knowledge DBs without the column give
// an empty map.
func loadRecipeUnlocks(db *sql.DB) (map[string][]MissionRef, error) {
	rows, err := db.Query(`
		SELECT DISTINCT c.template_id, COALESCE(t.title, c.title), COALESCE(t.type, ''), c.recipes_unlocked
		FROM mission_completions c LEFT JOIN mission_templates t ON t.id = c.template_id
		WHERE c.recipes_unlocked NOT IN ('', '[]')`)
	if err != nil {
		return map[string][]MissionRef{}, nil //nolint:nilerr // pre-unlock knowledge DB: nothing unlocked yet
	}
	defer func() { _ = rows.Close() }()
	out := map[string][]MissionRef{}
	for rows.Next() {
		var m MissionRef
		var unlocked string
		if err := rows.Scan(&m.ID, &m.Title, &m.Type, &unlocked); err != nil {
			return nil, err
		}
		var ids []string
		if err := json.Unmarshal([]byte(unlocked), &ids); err != nil {
			continue
		}
		for _, id := range ids {
			if !slices.Contains(out[id], m) {
				out[id] = append(out[id], m)
			}
		}
	}
	return out, rows.Err()
}

// buildUnlisted merges the captured details with the in-game unlocks: every
// unlocked recipe is listed, with a placeholder when its details are not
// captured yet. Items link to their catalog page when one exists.
func buildUnlisted(captured []*UnlistedRecipe, unlocks map[string][]MissionRef, recipes map[string]*Recipe, items map[string]*Item) []*UnlistedRecipe {
	byID := map[string]*UnlistedRecipe{}
	for _, r := range captured {
		byID[r.ID] = r
	}
	for id := range unlocks {
		if byID[id] == nil {
			byID[id] = &UnlistedRecipe{ID: id, Name: wordsTitle(id), Missing: true}
		}
	}
	link := func(its []UnlistedItem) {
		for i, it := range its {
			if item := items[it.ItemID]; item != nil && item.Category != "" {
				its[i].URL = fmt.Sprintf("../items/%s/%s.html", item.Category, it.ItemID)
			}
		}
	}
	out := make([]*UnlistedRecipe, 0, len(byID))
	for id, r := range byID {
		r.UnlockedBy = unlocks[id]
		if rec := recipes[id]; rec != nil && rec.Category != "" {
			r.CatalogURL = fmt.Sprintf("%s/%s.html", dirName(rec.Category), id)
		}
		link(r.Inputs)
		link(r.Outputs)
		out = append(out, r)
	}
	slices.SortFunc(out, func(a, b *UnlistedRecipe) int {
		return cmp.Or(cmp.Compare(boolInt(a.Missing), boolInt(b.Missing)), cmp.Compare(a.Name, b.Name))
	})
	return out
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// generateUnlistedPage writes kb/recipes/unlisted.html. Failures are warnings:
// the page is a side page and must not break a full regeneration.
func generateUnlistedPage(recipes map[string]*Recipe, items map[string]*Item) {
	captured, err := loadUnlistedFile("data/recipes_unlisted.json")
	if err != nil {
		log.Printf("warning: unlisted recipes: %v (page will be skipped)", err)
		return
	}
	unlocks := map[string][]MissionRef{}
	if kdb, err := sql.Open("sqlite", "../spacemolt-knowledge.db"); err == nil {
		if unlocks, err = loadRecipeUnlocks(kdb); err != nil {
			log.Printf("warning: unlisted recipes: mission unlocks: %v", err)
			unlocks = map[string][]MissionRef{}
		}
		_ = kdb.Close()
	}
	list := buildUnlisted(captured, unlocks, recipes, items)
	f, err := os.Create(filepath.Join("kb/recipes", "unlisted.html"))
	if err != nil {
		log.Printf("warning: unlisted recipes: %v", err)
		return
	}
	defer func() { _ = f.Close() }()
	if err := renderUnlistedPage(f, list); err != nil {
		log.Printf("warning: unlisted recipes: %v", err)
		return
	}
	fmt.Printf("Generated unlisted recipes page in kb/recipes/unlisted.html (%d recipes)\n", len(list))
}

func renderUnlistedPage(w io.Writer, list []*UnlistedRecipe) error {
	t, err := htmltpl.New("unlisted").Funcs(htmltpl.FuncMap{
		"secs": func(f float64) string { return fmt.Sprintf("%.1f", f) },
	}).Parse(unlistedTemplate)
	if err != nil {
		return err
	}
	return t.Execute(w, list)
}

var unlistedTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Unlisted Recipes - Spacemolt KB</title>
    <link rel="stylesheet" href="../smui.css">
    <link rel="stylesheet" href="../recipes/recipes.css">
    <style>
    .ul-recipe { margin-top: 1.5rem; }
    .ul-recipe h3 { margin-bottom: 0.25rem; }
    .ul-meta { font-size: 13px; color: hsl(var(--muted-foreground)); }
    .ul-badge { display: inline-block; font-size: 11px; padding: 1px 6px; border-radius: 4px; border: 1px solid hsl(var(--border)); margin-left: 6px; vertical-align: middle; }
    .ul-missing { padding: 0.75rem 1rem; border: 1px dashed hsl(var(--border)); color: hsl(var(--muted-foreground)); }
    </style>
</head>
<body>
` + siteHeader + `
    <main class="container page-content">
        <div class="breadcrumb"><a href="index.html">Recipes</a> / Unlisted</div>
        <h2>Unlisted Recipes</h2>
        <p class="text-muted mt-1">Recipes that a mission teaches and the public catalog does not list. Looking one up in the catalog returns "not found", so the details below come from the unlocking pilot's own crafting quote. Times are that pilot's skill-adjusted time per run, not the base time.</p>
{{- range .}}
        <section class="ul-recipe" id="{{.ID}}">
            <h3>{{.Name}}{{if .CatalogURL}}<a class="ul-badge" href="{{.CatalogURL}}">also in catalog</a>{{end}}</h3>
{{- if .UnlockedBy}}
            <div class="ul-meta">Unlocked by completing {{range $i, $m := .UnlockedBy}}{{if $i}}, {{end}}{{if .URL}}<a href="{{.URL}}">{{.Title}}</a>{{else}}{{.Title}}{{end}}{{end}}</div>
{{- end}}
{{- if .Missing}}
            <div class="ul-missing">Unlocked in-game, but its inputs and outputs have not been captured yet. Run <code>craft {{.ID}} --dry_run=true</code> as a pilot who has it and add the quote to <code>data/recipes_unlisted.json</code>.</div>
{{- else}}
{{- if .Note}}
            <p class="mt-1">{{.Note}}</p>
{{- end}}
            <div class="mt-1"><b>Makes</b> {{range $i, $o := .Outputs}}{{if $i}}, {{end}}{{.Quantity}} × {{if .URL}}<a href="{{.URL}}">{{.Name}}</a>{{else}}{{.Name}} <span class="text-muted">(not in catalog)</span>{{end}}{{end}}</div>
            <table>
                <thead><tr><th>Input</th><th class="r">Qty</th></tr></thead>
                <tbody>
{{- range .Inputs}}
                    <tr><td>{{if .URL}}<a href="{{.URL}}">{{.Name}}</a>{{else}}{{.Name}} <span class="text-muted">(not in catalog)</span>{{end}}</td><td class="r">{{.Quantity}}</td></tr>
{{- end}}
                </tbody>
            </table>
            <div class="ul-meta">{{if .Venue}}{{.Venue}} · {{end}}{{if .TimePerRun}}{{secs .TimePerRun}} ticks per run (skill-adjusted){{end}}{{if .Captured}} · quoted {{.Captured}}{{if .CapturedBy}} by {{.CapturedBy}}{{end}}{{end}}</div>
{{- end}}
        </section>
{{- end}}
    </main>
` + themeScript + `
</body>
</html>
`

// wordsTitle turns a recipe id into a display name, capitalizing every word:
// assemble_lane_anchor_prototype → Assemble Lane Anchor Prototype.
func wordsTitle(id string) string {
	words := strings.Fields(strings.ReplaceAll(id, "_", " "))
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}
