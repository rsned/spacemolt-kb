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
		">2.9<", // 1 flex_polymer/cycle at a 3,000-tick cycle = 2.88/day
		"3,000 ticks", "player-reported 2026-10-26",
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
	guide := "---\ntitle: Shed\nshow_per_day: true\nsteps:\n  - title: Shed\n    facility: bare_shed\n---\n"
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
	side := html[strings.Index(html, `class="guide-sidebar`):]
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

// Without maintenance_cycle_ticks the guide falls back to the default cycle
// and says so, rather than silently presenting it as measured.
func TestGenerate_DefaultCycleLabelled(t *testing.T) {
	dir := t.TempDir()
	guide := "---\ntitle: Shed\nshow_per_day: true\nsteps:\n  - title: Bay\n    facility: locker_bay\n---\n"
	if err := os.WriteFile(filepath.Join(dir, "shed.md"), []byte(guide), 0o644); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if _, err := generate(dir, testSnapshots(t), out); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(out, "shed.html"))
	for _, want := range []string{"3,000 ticks", "default", ">2.9<"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("missing %q", want)
		}
	}
}

// The guide page carries an on-this-page TOC: the intro, every build step and
// every reference-section heading, each pointing at an id that exists.
func TestGenerate_TOCListsStepsAndSections(t *testing.T) {
	html, _ := runFixture(t)
	toc := html[strings.Index(html, `class="guide-toc"`):]
	toc = toc[:strings.Index(toc, "</nav>")]
	for _, want := range []string{`href="#top"`, `href="#step-0"`, `href="#step-5"`, `href="#reference"`, ">Reference<", ">Step 1: Locker Bay<"} {
		if !strings.Contains(toc, want) {
			t.Errorf("TOC missing %q", want)
		}
	}
	for _, id := range []string{`id="top"`, `id="step-0"`, `id="reference"`} {
		if !strings.Contains(html, id) {
			t.Errorf("page missing anchor %s", id)
		}
	}
}

// Founding facts render as a label/value list with the basis under the value,
// not a three-column table whose narrow columns wrap every word.
func TestGenerate_FactsAsList(t *testing.T) {
	html, _ := runFixture(t)
	for _, want := range []string{`<dl class="facts">`, "<dt>Founding fee</dt>", `<span class="basis">measured 2026-10-25</span>`} {
		if !strings.Contains(html, want) {
			t.Errorf("guide missing %q", want)
		}
	}
	if strings.Contains(html, `<table class="facts">`) {
		t.Error("facts still rendered as a table")
	}
}

// Station upkeep lives in a drawer opened from an edge tab, and the guide's
// summary sits under the title as its lede.
func TestGenerate_UpkeepDrawerAndLede(t *testing.T) {
	html, _ := runFixture(t)
	for _, want := range []string{`class="guide-sidebar guide-drawer"`, `class="drawer-tab"`, `class="drawer-close"`,
		`<p class="guide-lede">A fixture guide for the generator tests.</p>`} {
		if !strings.Contains(html, want) {
			t.Errorf("guide missing %q", want)
		}
	}
}

// A step's break_before renders as a divider just before that step's card,
// so a guide can mark where a strict build order gives way to a flexible one.
func TestGenerate_BreakBeforeStep(t *testing.T) {
	html, _ := runFixture(t)
	brk := strings.Index(html, `<div class="step-break">`)
	card := strings.Index(html, `id="step-5"`)
	if brk < 0 || card < 0 || brk > card {
		t.Fatalf("step-break at %d, step-5 card at %d: want a break just before step 5", brk, card)
	}
	between := html[brk:card]
	for _, want := range []string{"<strong>From here on the order is flexible.</strong>", `href="../facilities/service/locker_bay.html"`} {
		if !strings.Contains(between, want) {
			t.Errorf("break missing %q in %q", want, between)
		}
	}
	if strings.Count(html, `<div class="step-break">`) != 1 {
		t.Error("want exactly one step break")
	}
}

// Until the maintenance cycle length is measured, a guide shows upkeep per
// cycle only: show_per_day defaults to false, and no /day figure, column or
// heading appears anywhere on the page or in the upkeep panel.
func TestGenerate_PerCycleOnlyByDefault(t *testing.T) {
	src, err := os.ReadFile("testdata/guides/test-guide.md")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	body := strings.Replace(string(src), "show_per_day: true\n", "", 1)
	if err := os.WriteFile(filepath.Join(dir, "test-guide.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if _, err := generate(dir, testSnapshots(t), out); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, "test-guide.html"))
	if err != nil {
		t.Fatal(err)
	}
	html := string(b)
	for _, bad := range []string{"/day", "units/day", "per day"} {
		if strings.Contains(html, bad) {
			t.Errorf("per-cycle-only guide still shows %q", bad)
		}
	}
	for _, want := range []string{">/cycle<", "Station upkeep / cycle", "upkeep 55/cycle", "units/cycle"} {
		if !strings.Contains(html, want) {
			t.Errorf("per-cycle-only guide missing %q", want)
		}
	}
}
