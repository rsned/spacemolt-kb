package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDistanceSystems(t *testing.T) {
	systems := []*System{
		{ID: "zeta", Name: "zeta", PositionX: 3.04, PositionY: 4},
		{ID: "sol", Name: "Sol"},
		{ID: "lost", Name: "Lost", PositionUnknown: true},
		{ID: "seen", Name: "Seen", Sighted: true},
		{ID: "est", Name: "Estimate", PositionX: -10, PositionY: 5, PositionApprox: true},
	}
	got := distanceSystems(systems)
	var names []string
	for _, s := range got {
		names = append(names, s.Name)
	}
	if strings.Join(names, ",") != "Estimate,Sol,zeta" {
		t.Fatalf("names = %v, want Estimate,Sol,zeta (case-insensitive order, unknown/sighted dropped)", names)
	}
	if !got[0].Approx || got[1].Approx {
		t.Errorf("approx flags wrong: %+v", got)
	}
	if got[2].X != 3 {
		t.Errorf("x = %v, want rounded to 0.1 (3)", got[2].X)
	}
}

func TestWriteDistancePage(t *testing.T) {
	dir := t.TempDir()
	systems := []*System{{ID: "sol", Name: "Sol"}, {ID: "a<b", Name: "A</script>"}}
	if err := writeDistancePage(dir, systems); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "distance.html"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{"Distance Calculator", `"id":"sol"`, " 1235", "nav.js"} {
		if !strings.Contains(s, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if strings.Contains(s, "A</script>") {
		t.Error("system name not escaped inside the script block")
	}
}
