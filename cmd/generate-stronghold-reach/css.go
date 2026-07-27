package main

import (
	"fmt"
	"strings"
)

// Reach map palette.
const (
	dotDim         = "#3a4557"
	dotInReach     = "#f0f4f8"
	dotStronghold  = "#ff2d2d"
)

// ReachCSS generates the frame-reveal rules for radii 1..maxRadius.
//
// Radius 0 is excluded on purpose: the strongholds are in reach at every
// frame, so their rb-0 geometry is never hidden and their sr-0 dots get a
// single static rule instead of one per frame.
func ReachCSS(maxRadius int) string {
	var b strings.Builder

	// Dots default to dim; strongholds are always red.
	fmt.Fprintf(&b, "#reach-map .galaxy-sys-dot{fill:%s;}\n", dotDim)
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
