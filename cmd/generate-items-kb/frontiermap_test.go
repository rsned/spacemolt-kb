package main

import (
	"fmt"
	"strings"
	"testing"
	"time"
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
		mk("spur", "Spur", 7292, -2666, true, ""),
	}
}

func TestCompassWedge(t *testing.T) {
	// In-game orientation: +y is down, so north is -y.
	cases := []struct {
		x, y float64
		want string
	}{
		{1, 0, "East"}, {0, -1, "North"}, {-1, 0, "West"}, {0, 1, "South"},
		{1, -1, "North-east"}, {-1, 1, "South-west"}, {1, 1, "South-east"}, {-1, -1, "North-west"},
		{1, -0.39, "East"}, {1, -0.42, "North-east"}, // either side of 22.5°
	}
	for _, c := range cases {
		if got := compassNames[compassWedge(c.x, c.y)]; got != c.want {
			t.Errorf("compassWedge(%v, %v) = %s, want %s", c.x, c.y, got, c.want)
		}
	}
}

func TestFrontierWedges(t *testing.T) {
	wedges, inner := frontierWedges(frontierFixture())
	got := map[string]int{}
	for i, w := range wedges {
		got[compassNames[i]] = len(w)
	}
	want := map[string]int{"South-west": 2, "North-west": 5, "East": 1}
	for name, n := range want {
		if got[name] != n {
			t.Errorf("wedge %s has %d systems, want %d (all: %v)", name, got[name], n, got)
		}
	}
	if len(inner) != 1 || inner[0].ID != "the_sanctum" {
		t.Errorf("inner = %v, want The Sanctum only", inner)
	}
}

func TestFrontierCalloutsSVG(t *testing.T) {
	svg := renderFrontierCallouts(frontierFixture())
	for _, want := range []string{"Ash Pearl", `href="ash_pearl/"`, "North-west", "old rim", `class="fc-inner"`, "The Sanctum"} {
		if !strings.Contains(svg, want) {
			t.Errorf("compass map lacks %q", want)
		}
	}
	if n := strings.Count(svg, `class="fc-panel"`); n != 8 {
		t.Errorf("panels = %d, want 8", n)
	}
	// North, North-east, South-east, South and West hold nothing.
	if n := strings.Count(svg, `class="fc-empty"`); n != 5 {
		t.Errorf("empty wedges = %d, want 5", n)
	}
	if !strings.Contains(svg, `class="fc-lane"`) {
		t.Error("lanes inside a wedge should be drawn")
	}
}

func TestFrontierCalloutsEmpty(t *testing.T) {
	if svg := renderFrontierCallouts([]*System{{ID: "sol", Name: "Sol"}}); svg != "" {
		t.Errorf("no frontier systems should draw nothing, got %d bytes", len(svg))
	}
}

// Labels exactly one line apart once made the nudge loop spin forever on
// floating-point rounding; this must finish.
func TestWedgePanelLabelsOneLineApart(t *testing.T) {
	var sys []*System
	for i := range 6 {
		sys = append(sys, &System{ID: fmt.Sprintf("s%d", i), Name: fmt.Sprintf("S%d", i), PositionX: 100, PositionY: 7000 + float64(i)*0.1})
	}
	var b strings.Builder
	done := make(chan struct{})
	go func() {
		writeWedgePanel(&b, sys, 0.3, 0.7, 323.1, 280)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("label placement did not finish")
	}
}
