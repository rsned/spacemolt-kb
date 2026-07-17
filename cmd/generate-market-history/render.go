package main

import (
	"embed"
	"html/template"
	"os"
	"path/filepath"
)

//go:embed templates/*.tmpl
var tmplFS embed.FS

// renderMarketIndex writes the landing page: a legend and one card per category.
func renderMarketIndex(outDir string, cards []CategoryCard) error {
	t, err := template.ParseFS(tmplFS, "templates/market-index.html.tmpl")
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
	return t.Execute(f, map[string]any{"Cards": cards, "LastUpdated": lastMarketUpdate})
}

// renderCategoryPage writes outDir/<category>/index.html, rendering each item's
// candlestick chart inline.
func renderCategoryPage(outDir string, page CategoryPage) error {
	page.LastUpdated = lastMarketUpdate
	for i := range page.Items {
		page.Items[i].Chart = candlestickSVG(page.Items[i].Candles)
	}
	t, err := template.ParseFS(tmplFS, "templates/market-category.html.tmpl")
	if err != nil {
		return err
	}
	dir := filepath.Join(outDir, page.Category)
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
