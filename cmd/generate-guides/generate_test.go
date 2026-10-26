package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const testFacilitiesJSON = `{"items":[
 {"id":"locker_bay","name":"Locker Bay","category":"service","service_type":"storage","build_cost":500000,"build_time":80,
  "build_materials":[{"item_id":"steel_plate","quantity":1600}],
  "maintenance_inputs":[{"item_id":"steel_plate","quantity":1},{"item_id":"flex_polymer","quantity":1}]},
 {"id":"atmospheric_recycler","name":"Atmospheric Recycler","category":"infrastructure","build_cost":600000,"build_time":150,
  "build_materials":[{"item_id":"steel_plate","quantity":1200}],
  "maintenance_inputs":[{"item_id":"water_ice","quantity":3}],"life_support_supply":12},
 {"id":"power_hog","name":"Power Hog","category":"production","build_cost":1000,"power_draw":6},
 {"id":"stellar_collector_grid","name":"Stellar Collector Grid","category":"infrastructure","build_cost":900000,"build_time":250,
  "maintenance_inputs":[{"item_id":"circuit_board","quantity":2}],"power_supply":20},
 {"id":"bunker_fed_reactor","name":"Bunker-Fed Reactor","category":"infrastructure","build_cost":500000,"maintenance_fuel":55,"power_supply":12},
 {"id":"bare_shed","name":"Bare Shed","category":"service","build_cost":10}
]}`

const testItemsJSON = `{"items":[
 {"id":"steel_plate","name":"Steel Plate","category":"refined"},
 {"id":"flex_polymer","name":"Flex Polymer","category":"refined"},
 {"id":"water_ice","name":"Water Ice","category":"ore"},
 {"id":"circuit_board","name":"Circuit Board","category":"component"}
]}`

func testSnapshots(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "20261018")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"catalog_facilities.json": testFacilitiesJSON, "catalog_items.json": testItemsJSON} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func runFixture(t *testing.T) (string, []string) {
	t.Helper()
	out := t.TempDir()
	warnings, err := generate("testdata/guides", testSnapshots(t), out)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, "test-guide.html"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b), warnings
}

func TestGenerate_WritesGuideAndIndex(t *testing.T) {
	out := t.TempDir()
	if _, err := generate("testdata/guides", testSnapshots(t), out); err != nil {
		t.Fatal(err)
	}
	idx, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`href="test-guide.html"`, "Test Station Guide", "A fixture guide for the generator tests."} {
		if !strings.Contains(string(idx), want) {
			t.Errorf("index missing %q", want)
		}
	}
}

func TestGenerate_CardShowsCostsAndLinks(t *testing.T) {
	html, _ := runFixture(t)
	for _, want := range []string{
		`href="../facilities/service/locker_bay.html"`,
		"500,000", "1,600",
		`href="../items/refined/flex_polymer.html"`,
		">86<", // 1 flex_polymer/cycle = 86/day
		"Gets its own POI.",
		"5,000,000 cr", "measured 2026-10-25",
		"Reference",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("guide missing %q", want)
		}
	}
}

func TestGenerate_NoUpkeepText(t *testing.T) {
	dir := t.TempDir()
	guide := "---\ntitle: Shed\nsteps:\n  - title: Shed\n    facility: bare_shed\n---\n"
	if err := os.WriteFile(filepath.Join(dir, "shed.md"), []byte(guide), 0o644); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if _, err := generate(dir, testSnapshots(t), out); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(out, "shed.html"))
	if !strings.Contains(string(b), "No upkeep") {
		t.Error("facility without upkeep should say No upkeep")
	}
}

func TestGenerate_SidebarTotalsAndDeficit(t *testing.T) {
	html, _ := runFixture(t)
	side := html[strings.Index(html, `class="guide-sidebar"`):]
	if !strings.Contains(side, "12 / 0") {
		t.Errorf("sidebar should show life support 12 / 0")
	}
	if !strings.Contains(html, "⚠") {
		t.Error("power_hog step should be flagged as a power deficit")
	}
	if !strings.Contains(side, "20 / 6") {
		t.Errorf("sidebar should end with power 20 / 6")
	}
}

func TestGenerate_MeasuredShownBesideCatalog(t *testing.T) {
	html, _ := runFixture(t)
	if !strings.Contains(html, "measured (2026-10-26)") || !strings.Contains(html, "storage diff") {
		t.Error("measured block missing")
	}
	if !strings.Contains(html, `href="../facilities/infrastructure/bunker_fed_reactor.html"`) {
		t.Error("alternative not shown")
	}
}

func TestGenerate_UnknownFacilityFails(t *testing.T) {
	dir := t.TempDir()
	guide := "---\ntitle: Broken\nsteps:\n  - title: Mystery Step\n    facility: no_such_facility\n---\n"
	if err := os.WriteFile(filepath.Join(dir, "test-guide.md"), []byte(guide), 0o644); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	_, err := generate(dir, testSnapshots(t), out)
	if err == nil || !strings.Contains(err.Error(), "test-guide.md") || !strings.Contains(err.Error(), "Mystery Step") {
		t.Fatalf("err = %v, want file name and step title", err)
	}
	if _, serr := os.Stat(filepath.Join(out, "test-guide.html")); !os.IsNotExist(serr) {
		t.Error("a failing guide must write nothing")
	}
}

func TestGenerate_UnknownRefWarns(t *testing.T) {
	html, warnings := runFixture(t)
	if !slices.ContainsFunc(warnings, func(w string) bool { return strings.Contains(w, "nope") }) {
		t.Errorf("warnings = %v, want one naming nope", warnings)
	}
	if strings.Contains(html, `nope.html`) {
		t.Error("unknown ref rendered as a link")
	}
}

// The reference section carries the station command table; markdown tables
// must render as tables, while raw HTML stays stripped (safe mode).
func TestGenerate_MarkdownTablesSafe(t *testing.T) {
	dir := t.TempDir()
	guide := "---\ntitle: T\nsteps: []\n---\n| Action | Params |\n|---|---|\n| `info` | none |\n\n<script>alert(1)</script>\n"
	if err := os.WriteFile(filepath.Join(dir, "t.md"), []byte(guide), 0o644); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if _, err := generate(dir, testSnapshots(t), out); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(out, "t.html"))
	body := string(b)[strings.Index(string(b), `class="guide-body"`):]
	if !strings.Contains(body, "<table>") || !strings.Contains(body, "<code>info</code>") {
		t.Error("markdown table not rendered as a table")
	}
	if strings.Contains(body, "alert(1)") {
		t.Error("raw HTML leaked through")
	}
}

// A guide that was deleted or renamed must not keep publishing its old page
// (and its old catalog numbers): outDir is the generator's own directory.
func TestGenerate_RemovesStalePages(t *testing.T) {
	out := t.TempDir()
	for name, body := range map[string]string{"old-guide.html": "stale", "notes.txt": "keep me"} {
		if err := os.WriteFile(filepath.Join(out, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := generate("testdata/guides", testSnapshots(t), out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "old-guide.html")); !os.IsNotExist(err) {
		t.Error("stale old-guide.html was left in place")
	}
	if _, err := os.Stat(filepath.Join(out, "notes.txt")); err != nil {
		t.Error("non-HTML files must be left alone")
	}
}
