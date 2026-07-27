package main

import (
	"regexp"
	"strings"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/galaxymap"
)

// rbClassRE matches every "rb-N" class galaxymap.Render's ReachBlob
// geometry can emit.
var rbClassRE = regexp.MustCompile(`class="rb-(\d+)"`)

// TestEmittedBlobClassesHaveMatchingCSSRules renders a small synthetic
// galaxy through galaxymap.Render with a ReachBlob and checks every rb-N
// class it emits has a matching selector in ReachCSS's output. The rb-/sr-
// naming contract spans three files (galaxymap.go emits it, main.go
// consumes it for sr-, css.go selects it) with no other test crossing that
// boundary — a rename on one side alone would ship a page with invisible
// blobs through an otherwise green test run.
func TestEmittedBlobClassesHaveMatchingCSSRules(t *testing.T) {
	systems := []*galaxymap.System{
		{
			ID: "sol", Name: "Sol", PositionX: 0, PositionY: 0, IsStronghold: true,
			Connections: []galaxymap.Connection{{SystemID: "vega", Distance: 10}},
		},
		{
			ID: "vega", Name: "Vega", PositionX: 100, PositionY: 0,
			Connections: []galaxymap.Connection{
				{SystemID: "sol", Distance: 10},
				{SystemID: "rigel", Distance: 10},
			},
		},
		{
			ID: "rigel", Name: "Rigel", PositionX: 200, PositionY: 0,
			Connections: []galaxymap.Connection{{SystemID: "vega", Distance: 10}},
		},
	}
	index := make(map[string]*galaxymap.System, len(systems))
	for _, s := range systems {
		index[s.ID] = s
	}
	dist := map[string]int{"sol": 0, "vega": 1, "rigel": 2}
	const maxRadius = 2

	svg := galaxymap.Render(systems, nil, index, galaxymap.Options{
		ShowConnections: true,
		ReachBlob: &galaxymap.ReachBlob{
			Radius: func(id string) int {
				if d, ok := dist[id]; ok {
					return d
				}
				return -1
			},
			Max:   maxRadius,
			Color: "#c53030",
		},
	})

	matches := rbClassRE.FindAllStringSubmatch(svg, -1)
	if len(matches) == 0 {
		t.Fatalf("no rb-N classes found in rendered SVG:\n%s", svg)
	}

	css := ReachCSS(maxRadius)

	seen := make(map[string]bool, len(matches))
	for _, m := range matches {
		seen[m[1]] = true
	}
	for n := range seen {
		if n == "0" {
			continue // always-visible geometry, intentionally has no frame rule
		}
		selector := "#reach-map .rb-" + n
		if !strings.Contains(css, selector) {
			t.Errorf("emitted class rb-%s has no matching CSS rule %q in:\n%s", n, selector, css)
		}
	}
}
