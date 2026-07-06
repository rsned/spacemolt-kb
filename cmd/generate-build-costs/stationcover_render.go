package main

import (
	"html/template"
	"os"
	"path/filepath"
)

// renderStationCover writes the station-cover did-you-know page to outPath.
func renderStationCover(outPath string, p stationCoverPage) error {
	t, err := template.ParseFS(tmplFS, "templates/stationcover.html.tmpl")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return t.Execute(f, p)
}
