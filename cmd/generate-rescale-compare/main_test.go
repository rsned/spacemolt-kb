package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const samplePage = `<h2 class="sys-name">Sol</h2>
<div class="section-label">Points of Interest (3)</div>
<table><thead><tr><th></th><th>Name</th></tr></thead>
<tbody>
<tr><td>☀</td><td>Sol Star</td><td>Sun</td><td>G2V</td><td>0.0</td><td>(0.0, 0.0)</td><td>—</td><td>—</td><td>x</td></tr>
<tr><td>●</td><td><a href="p.html">Neptune</a></td><td>Planet</td><td><span class="text-muted">—</span></td><td>6.7</td><td>(4.3, -5.1)</td><td>—</td><td>—</td><td>x</td></tr>
<tr><td>❄</td><td>Kuiper Ice Fields</td><td>Ice Field</td><td>—</td><td>7.0</td><td>(6.6, 2.4)</td><td>—</td><td>—</td><td>x</td></tr>
</tbody></table>`

func TestParseSystemPage(t *testing.T) {
	l, err := parseSystemPage(samplePage)
	if err != nil {
		t.Fatal(err)
	}
	if l.Name != "Sol" || len(l.Bodies) != 3 {
		t.Fatalf("layout = %+v", l)
	}
	if b := l.Bodies[0]; b.Type != "sun" || b.Class != "G2V" {
		t.Errorf("sun = %+v", b)
	}
	if b := l.Bodies[1]; b.Name != "Neptune" || b.X != 4.3 || b.Y != -5.1 || b.Class != "" {
		t.Errorf("neptune = %+v", b)
	}
	if b := l.Bodies[2]; b.Type != "ice_field" {
		t.Errorf("ice field type = %q", b.Type)
	}
}

func TestCompareSystems(t *testing.T) {
	before := map[string]Layout{
		"sol":   {Name: "Sol", Bodies: []Body{{Name: "Sol Star", Type: "sun", Class: "G2V"}, {Name: "Neptune", Type: "planet", X: 4.3, Y: -5.1}, {Name: "Old Relic", Type: "relic", X: 1}}},
		"alfa":  {Name: "Alfa", Bodies: []Body{{Name: "Alfa I", Type: "planet", X: 3}}},
		"bravo": {Name: "Bravo", Bodies: []Body{{Name: "Bravo I", Type: "planet", X: 3}}},
	}
	after := map[string]afterLayout{
		"sol":  {Layout: Layout{Name: "Sol", Bodies: []Body{{Name: "Sol Star", Type: "sun", Class: "G2V"}, {Name: "Neptune", Type: "planet", X: 19.4, Y: -23.01}, {Name: "New Gate", Type: "station", X: 2}}}, rescanned: true},
		"alfa": {Layout: Layout{Name: "Alfa", Bodies: []Body{{Name: "Alfa I", Type: "planet", X: 3}}}, rescanned: false},
	}
	got := compareSystems(before, after)
	if len(got) != 3 || got[0].ID != "alfa" || got[2].ID != "sol" {
		t.Fatalf("order = %v %v %v", got[0].ID, got[1].ID, got[2].ID)
	}
	if got[0].Rescanned || got[1].Rescanned || !got[2].Rescanned {
		t.Fatal("only sol is rescanned")
	}
	sol := got[2]
	if sol.Class != "G2V" || sol.AfterExtent < 30 || sol.BeforeExtent > 7 {
		t.Errorf("sol = %+v", sol)
	}
	var names []string
	for _, r := range sol.Rows {
		names = append(names, r.Name+":"+funcs["ratio"].(func(BodyRow) string)(r))
	}
	if strings.Join(names, ",") != "Sol Star:,Old Relic:gone,New Gate:new,Neptune:×4.5" {
		t.Errorf("rows = %v", names)
	}
	if got := funcs["ratio"].(func(BodyRow) string)(BodyRow{Name: "Mercury", Type: "planet", Before: 0.41, After: 0.39, HasB: true, HasA: true}); got != "" {
		t.Errorf("rounding-sized move shows %q", got)
	}
}

func TestWritePages(t *testing.T) {
	dir := t.TempDir()
	systems := compareSystems(
		map[string]Layout{"sol": {Name: "Sol", Bodies: []Body{{Name: "Sol Star", Type: "sun"}, {Name: "Earth", Type: "planet", X: 1}}},
			"x": {Name: "Xeno", Bodies: []Body{{Name: "Xeno I", Type: "planet", X: 2}}}},
		map[string]afterLayout{"sol": {Layout: Layout{Bodies: []Body{{Name: "Sol Star", Type: "sun"}, {Name: "Earth", Type: "planet", X: 1}}}, rescanned: true}})
	if err := os.WriteFile(filepath.Join(dir, "stale.html"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := writePages(dir, systems)
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "stale.html")); !os.IsNotExist(err) {
		t.Error("stale page left behind")
	}
	idx, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	for _, want := range []string{`href="sol.html"`, "awaiting rescan", "Xeno", "1 of 2 systems rescanned"} {
		if !strings.Contains(string(idx), want) {
			t.Errorf("index lacks %q", want)
		}
	}
	page, _ := os.ReadFile(filepath.Join(dir, "sol.html"))
	for _, want := range []string{`class="rc-canvas"`, "BEFORE", "AFTER", "Earth"} {
		if !strings.Contains(string(page), want) {
			t.Errorf("detail page lacks %q", want)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "x.html")); !os.IsNotExist(err) {
		t.Error("unrescanned system got a page")
	}
}
