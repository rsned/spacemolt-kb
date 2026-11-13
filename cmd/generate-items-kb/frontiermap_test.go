package main

import (
	"strings"
	"testing"
)

func frontierFixture() []*System {
	mk := func(id, name string, x, y float64, uncharted bool, empire string) *System {
		return &System{ID: id, Name: name, PositionX: x, PositionY: y, Uncharted: uncharted, Empire: empire, PositionApprox: uncharted, LastUpdatedTick: 1}
	}
	sw1 := mk("ash_pearl", "Ash Pearl", -6259, 3300, true, "")
	sw2 := mk("split_lane", "Split Lane", -6012, 3660, true, "")
	sw1.Connections = []SystemConnection{{SystemID: "split_lane"}}
	return []*System{
		mk("sol", "Sol", 0, 0, false, "solarian"),
		mk("nexus_prime", "Nexus Prime", -4000, 2000, false, "voidborn"),
		sw1, sw2,
		mk("whitecap", "Whitecap", -4788, -6131, true, ""),
		mk("quiet_floor", "Quiet Floor", -4729, -6550, true, ""),
		mk("sallow", "Sallow", -4712, -7076, true, ""),
		mk("narrows", "Narrows", -4196, -7334, true, ""),
		mk("gloaming", "Gloaming", -4170, -7769, true, ""),
		mk("the_sanctum", "The Sanctum", -4478, 2345, true, ""),
	}
}

func TestFrontierRegions(t *testing.T) {
	regions := frontierRegions(frontierFixture())
	got := map[string]int{}
	for _, r := range regions {
		got[r.Name] = len(r.Systems)
	}
	// Chained within 1,000 units, so one region each; names use the
	// in-game orientation (+y is down), and inner regions take the empire
	// of the nearest charted system.
	want := map[string]int{"South-west rim": 2, "North-west rim": 5, "Inner Voidborn space": 1}
	for name, n := range want {
		if got[name] != n {
			t.Errorf("region %q has %d systems, want %d (all: %v)", name, got[name], n, got)
		}
	}
	if len(regions) != 3 {
		t.Errorf("regions = %d, want 3", len(regions))
	}
}

func TestFrontierCalloutsSVG(t *testing.T) {
	svg := renderFrontierCallouts(frontierFixture())
	for _, want := range []string{`class="fc-panel"`, "Ash Pearl", `href="ash_pearl/"`, "Inner Voidborn space", "old rim"} {
		if !strings.Contains(svg, want) {
			t.Errorf("callout map lacks %q", want)
		}
	}
	if n := strings.Count(svg, `class="fc-panel"`); n != len(frontierRegions(frontierFixture())) {
		t.Errorf("panels = %d", n)
	}
	if !strings.Contains(svg, `class="fc-lane"`) {
		t.Error("lanes inside a region should be drawn")
	}
}
