package main

import (
	"html/template"
	"os"
	"path/filepath"
)

// renderFacilitiesIndex writes the facility build-cost landing page: the group
// cards, a legend explaining the MKT-AVG vs Galaxy prices, and a per-category
// stats breakdown.
func renderFacilitiesIndex(outDir string, summaries []FacilityGroupSummary, stats []CategoryStat) error {
	t, err := template.ParseFS(tmplFS, "templates/facilities-index.html.tmpl")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(outDir, "index.html"))
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return t.Execute(f, map[string]any{"Summaries": summaries, "Stats": stats})
}

// renderFacilityGroup writes one group's page to outDir/<group>/index.html.
func renderFacilityGroup(outDir string, page FacilityGroupPage) error {
	t, err := template.ParseFS(tmplFS, "templates/facilities-group.html.tmpl")
	if err != nil {
		return err
	}
	dir := filepath.Join(outDir, page.Group)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(dir, "index.html"))
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return t.Execute(f, page)
}
