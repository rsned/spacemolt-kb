package main

import (
	"fmt"
	"strings"
)

// Reach map palette. Stronghold dots are orange rather than red so they
// stay distinct from the Crimson empire's own #DC143C.
const (
	dotDim        = "#3a4557"
	dotInReach    = "#f0f4f8"
	dotStronghold = "#ff9500"
)

// empireDotColors are the standard empire colors, matching the ones
// pkg/galaxymap paints on the main galaxy map. Systems belonging to an
// empire keep their colors until reach covers them, at which point the
// higher-specificity per-frame rule repaints them as in-reach.
//
// Keys are ordered explicitly rather than ranged over a map so the
// generated CSS is byte-stable across runs.
var empireDotColors = []struct{ Slug, Color string }{
	{"crimson", "#DC143C"},
	{"nebula", "#00CED1"},
	{"outerrim", "#2E8B57"},
	{"solarian", "#FFD700"},
	{"voidborn", "#9932CC"},
}

// ReachCSS generates the frame-reveal rules for radii 1..maxRadius.
//
// Radius 0 is excluded on purpose: the strongholds are in reach at every
// frame, so their rb-0 geometry is never hidden and their sr-0 dots get a
// single static rule instead of one per frame.
//
// The rb-N classes these selectors target are emitted by
// galaxymap.Render's ReachBlob geometry (pkg/galaxymap/galaxymap.go); the
// sr-N classes come from this package's HighlightClasses callback.
func ReachCSS(maxRadius int) string {
	var b strings.Builder

	// Dots default to dim; strongholds are always red.
	fmt.Fprintf(&b, "#reach-map .galaxy-sys-dot{fill:%s;}\n", dotDim)

	// Empire-owned systems show their own color while still out of reach.
	// These carry the same specificity as the dim rule above (id + class)
	// and win on source order; the per-frame rules below add an attribute
	// selector, so covering a system always outranks its empire color.
	for _, e := range empireDotColors {
		fmt.Fprintf(&b, "#reach-map .emp-%s{fill:%s;}\n", e.Slug, e.Color)
	}

	fmt.Fprintf(&b, "#reach-map .sr-0{fill:%s;}\n", dotStronghold)

	if maxRadius < 1 {
		return b.String()
	}

	// Every frame hidden by default.
	hide := make([]string, 0, maxRadius)
	for r := 1; r <= maxRadius; r++ {
		hide = append(hide, fmt.Sprintf("#reach-map .rb-%d", r))
	}
	fmt.Fprintf(&b, "%s{display:none;}\n", strings.Join(hide, ","))

	// Cumulative reveal per frame.
	for frame := 1; frame <= maxRadius; frame++ {
		blob := make([]string, 0, frame)
		dots := make([]string, 0, frame)
		for r := 1; r <= frame; r++ {
			blob = append(blob, fmt.Sprintf(`#reach-map[data-r="%d"] .rb-%d`, frame, r))
			dots = append(dots, fmt.Sprintf(`#reach-map[data-r="%d"] .sr-%d`, frame, r))
		}
		fmt.Fprintf(&b, "%s{display:inline;}\n", strings.Join(blob, ","))
		fmt.Fprintf(&b, "%s{fill:%s;}\n", strings.Join(dots, ","), dotInReach)
	}

	return b.String()
}
