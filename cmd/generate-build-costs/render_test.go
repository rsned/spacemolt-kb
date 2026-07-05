package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/buildcost"
)

func sampleMatrix() Matrix {
	targets := []buildcost.Target{{ID: "widget", Kind: "item", BoM: []buildcost.Requirement{{ItemID: "iron", Qty: 2}}}}
	books := map[string]*buildcost.Book{"st1": {Sell: map[string]buildcost.Ladder{"iron": {{Price: 10, Qty: 100}}}, BestBuy: map[string]float64{}}}
	stations := []StationMeta{{ID: "st1", Name: "Station One", Empire: "Sol"}}
	return BuildMatrix(targets, books, stations, map[string]string{"widget": "Widget"}, map[string]string{"widget": "Module"}, nil, nil)
}

func TestRenderIndex_WritesFileWithData(t *testing.T) {
	dir := t.TempDir()
	if err := renderIndex(dir, sampleMatrix()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{"Widget", "Station One", "Show only feasible", "BoM", "Recipe"} {
		if !strings.Contains(s, want) {
			t.Fatalf("index.html missing %q", want)
		}
	}
}

func TestMatrixJSON_Valid(t *testing.T) {
	js, err := matrixJSON(sampleMatrix())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(js, "widget") {
		t.Fatalf("json missing target id: %s", js)
	}
}

func TestRenderDetail_WritesTable(t *testing.T) {
	dir := t.TempDir()
	m := sampleMatrix()
	if err := renderDetail(dir, m.Rows[0], m.Stations); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "widget.html"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{"Widget", "Station One", "BoM", "Recipe", "Feasible"} {
		if !strings.Contains(s, want) {
			t.Fatalf("detail missing %q", want)
		}
	}
}
