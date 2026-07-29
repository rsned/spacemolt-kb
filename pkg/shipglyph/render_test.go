package shipglyph

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

func renderFixture(t *testing.T, s Stats) string {
	t.Helper()
	d := Infer(s)
	return Render(d, s, Options{Size: 200, ShowHardpoints: true, Title: s.Name})
}

func TestRenderProducesWellFormedSVG(t *testing.T) {
	out := renderFixture(t, Stats{ID: "war_wagon", Name: "War Wagon",
		Class: "Bulk Hauler", Faction: "crimson", Scale: 4, Weapon: 2, Defense: 2, Utility: 8})

	if !strings.HasPrefix(out, "<svg") {
		t.Errorf("output does not start with <svg>:\n%s", out[:min(120, len(out))])
	}
	if !strings.HasSuffix(strings.TrimSpace(out), "</svg>") {
		t.Errorf("output does not end with </svg>")
	}
	if !strings.Contains(out, `viewBox="0 0 200 200"`) {
		t.Errorf("missing or wrong viewBox")
	}
	if !strings.Contains(out, "<title>War Wagon</title>") {
		t.Errorf("missing accessible title")
	}
}

func TestRenderEmitsAllStableRegionIDs(t *testing.T) {
	out := renderFixture(t, Stats{ID: "comet", Name: "Comet",
		Class: "Liner", Faction: "nebula", Scale: 4, Defense: 4, Utility: 5})

	for _, name := range RegionNames {
		want := `id="region-` + name + `"`
		if !strings.Contains(out, want) {
			t.Errorf("missing %s", want)
		}
	}
	if !strings.Contains(out, `id="hull"`) {
		t.Errorf(`missing id="hull" group`)
	}
	if !strings.Contains(out, `id="hardpoints"`) {
		t.Errorf(`missing id="hardpoints" group`)
	}
}

func TestRenderDrawsAppendages(t *testing.T) {
	// A Nebula liner is inferred with swept wings; they must reach the SVG.
	out := renderFixture(t, Stats{ID: "comet", Name: "Comet",
		Class: "Liner", Faction: "nebula", Scale: 4, Defense: 4, Utility: 5})

	if !strings.Contains(out, `id="appendages"`) {
		t.Errorf(`missing id="appendages" group`)
	}
	if !strings.Contains(out, `id="ap-wing-1s"`) || !strings.Contains(out, `id="ap-wing-1p"`) {
		t.Errorf("missing per-side wing IDs")
	}
}

func TestRenderOmitsAppendageGroupWhenThereAreNone(t *testing.T) {
	d := Descriptor{
		Aspect: 3,
		Hull:   []HullPart{{Kind: "box", Span: [2]float64{0, 1}, Half: 0.2}},
	}
	out := Render(d, Stats{ID: "bare", Name: "Bare", Faction: "crimson"}, Options{Size: 200})

	if strings.Contains(out, `id="appendages"`) {
		t.Errorf("emitted an empty appendages group")
	}
}

func TestRenderEmitsHardpointIDs(t *testing.T) {
	out := renderFixture(t, Stats{ID: "magnate", Name: "Magnate",
		Class: "Command", Faction: "solarian", Scale: 4, Weapon: 3, Defense: 6, Utility: 5})

	for _, id := range []string{"hp-w1", "hp-w3", "hp-d6", "hp-u5"} {
		if !strings.Contains(out, `id="`+id+`"`) {
			t.Errorf("missing hardpoint %s", id)
		}
	}
}

func TestRenderGivesEachSlotKindItsOwnMark(t *testing.T) {
	// The three kinds must be told apart by silhouette, not only by class:
	// weapon is a circle, defense a square, utility a diamond. A shared
	// element for all three renders as undifferentiated scatter.
	out := renderFixture(t, Stats{ID: "magnate", Name: "Magnate",
		Class: "Command", Faction: "solarian", Scale: 4, Weapon: 1, Defense: 1, Utility: 1})

	for _, tc := range []struct{ id, element string }{
		{"hp-w1", "circle"},
		{"hp-d1", "rect"},
		{"hp-u1", "path"},
	} {
		if !strings.Contains(out, `<`+tc.element+` id="`+tc.id+`"`) {
			t.Errorf("%s is not a <%s>:\n%s", tc.id, tc.element, out)
		}
	}
}

func TestRenderUsesCurrentColorNotHardcodedColors(t *testing.T) {
	out := renderFixture(t, Stats{ID: "paradox", Name: "Paradox",
		Class: "Fighter", Faction: "voidborn", Scale: 1, Weapon: 2, Defense: 2, Utility: 1})

	if !strings.Contains(out, "currentColor") {
		t.Errorf("expected currentColor strokes for theme compatibility")
	}
	if strings.Contains(out, "#") || strings.Contains(out, "hsl(") {
		t.Errorf("glyph hardcodes a color; it must inherit from CSS:\n%s", out)
	}
}

func TestRenderScopesIDsWithPrefix(t *testing.T) {
	s := Stats{ID: "comet", Name: "Comet",
		Class: "Liner", Faction: "nebula", Scale: 4, Defense: 4, Utility: 5}
	d := Infer(s)

	prefixed := Render(d, s, Options{Size: 200, ShowHardpoints: true, Title: s.Name, IDPrefix: "abc-"})
	if !strings.Contains(prefixed, `id="abc-region-bow"`) {
		t.Errorf("missing prefixed region ID")
	}
	if !strings.Contains(prefixed, `id="abc-hull"`) {
		t.Errorf("missing prefixed hull ID")
	}
	if strings.Contains(prefixed, `id="region-bow"`) || strings.Contains(prefixed, `id="hull"`) {
		t.Errorf("unprefixed IDs leaked through when IDPrefix was set")
	}

	unprefixed := Render(d, s, Options{Size: 200, ShowHardpoints: true, Title: s.Name})
	if !strings.Contains(unprefixed, `id="region-bow"`) {
		t.Errorf("missing unprefixed region ID")
	}
	if !strings.Contains(unprefixed, `id="hull"`) {
		t.Errorf("missing unprefixed hull ID")
	}
}

func TestRenderIsByteIdenticalAcrossRuns(t *testing.T) {
	s := Stats{ID: "yard_sale", Name: "Yard Sale",
		Class: "Salvager", Faction: "outerrim", Scale: 3, Defense: 1, Utility: 4}
	first := renderFixture(t, s)
	second := renderFixture(t, s)
	if first != second {
		t.Errorf("Render is not deterministic")
	}
}

func TestRenderHandlesZeroSlotShip(t *testing.T) {
	out := renderFixture(t, Stats{ID: "prayer", Name: "Prayer",
		Class: "Freighter", Faction: "outerrim", Scale: 1})

	if !strings.Contains(out, `id="hull"`) {
		t.Errorf("hull missing for a zero-slot ship")
	}
	if strings.Contains(out, `id="hp-`) {
		t.Errorf("zero-slot ship should have no hardpoint markers")
	}
}

func TestRenderEveryFactionProducesOutput(t *testing.T) {
	for _, f := range []string{"crimson", "nebula", "solarian", "outerrim", "voidborn", "pirate", ""} {
		s := Stats{ID: "x_" + f, Name: "X", Class: "Cruiser", Faction: f, Scale: 3, Weapon: 2}
		out := renderFixture(t, s)
		if !strings.Contains(out, "<path") {
			t.Errorf("faction %q produced no paths", f)
		}
	}
}

func TestRenderNeutralizesOverlaySuppliedKind(t *testing.T) {
	d := Descriptor{
		Aspect:     3,
		Hull:       []HullPart{{Kind: "box", Span: [2]float64{0, 1}, Half: 0.2}},
		Appendages: []Appendage{{Kind: `w"><script>`, At: 0.5, Span: 0.2, Side: "both"}},
	}
	out := Render(d, Stats{ID: "x", Name: "X", Faction: "crimson"}, Options{Size: 200})

	if strings.Contains(out, "<script>") {
		t.Errorf("overlay-supplied kind injected markup into the glyph")
	}
	// The kind should be sanitized; the raw unsanitized form should not appear
	// anywhere in the attributes. Check that the dangerous characters from
	// the input kind do not appear together in any SVG attribute context.
	if strings.Contains(out, `glyph-ap-w">`) || strings.Contains(out, `id="ap-w">`) {
		t.Errorf("overlay-supplied kind broke out of its attribute")
	}
}

// outlineXExtent parses the glyph's outline path back out of the rendered
// markup and returns the min and max SVG x coordinates.
func outlineXExtent(t *testing.T, svg string) (float64, float64) {
	t.Helper()
	i := strings.Index(svg, `id="region-outline"`)
	if i < 0 {
		t.Fatalf("no region-outline path in output")
	}
	j := strings.Index(svg[i:], ` d="`)
	if j < 0 {
		t.Fatalf("outline path has no d attribute")
	}
	start := i + j + 4
	end := strings.Index(svg[start:], `"`)
	if end < 0 {
		t.Fatalf("unterminated d attribute")
	}

	minX, maxX := math.Inf(1), math.Inf(-1)
	for k, tok := range strings.Fields(svg[start : start+end]) {
		if k%2 != 0 {
			continue // odd tokens are y coordinates
		}
		v, err := strconv.ParseFloat(strings.TrimLeft(tok, "ML"), 64)
		if err != nil {
			t.Fatalf("unparsable x token %q", tok)
		}
		minX = math.Min(minX, v)
		maxX = math.Max(maxX, v)
	}
	return minX, maxX
}

func TestRenderHonorsDeclaredAspect(t *testing.T) {
	// Aspect is length divided by maximum beam, so two descriptors declaring
	// the same Aspect must render to the same beam even when their half-width
	// maxima differ by 4x: half-widths describe shape, not scale.
	const size = 200.0
	s := Stats{ID: "aspect", Name: "Aspect", Faction: "crimson"}
	narrow := Descriptor{Aspect: 3, Hull: []HullPart{{Kind: "box", Span: [2]float64{0, 1}, Half: 0.10}}}
	wide := Descriptor{Aspect: 3, Hull: []HullPart{{Kind: "box", Span: [2]float64{0, 1}, Half: 0.40}}}

	nMin, nMax := outlineXExtent(t, Render(narrow, s, Options{Size: size}))
	wMin, wMax := outlineXExtent(t, Render(wide, s, Options{Size: size}))
	nBeam, wBeam := nMax-nMin, wMax-wMin

	if math.Abs(nBeam-wBeam) > 0.5 {
		t.Errorf("beam varies with half-width: narrow %.2f vs wide %.2f; Aspect must govern proportion", nBeam, wBeam)
	}
	want := size * (1 - 2*glyphMargin) / 3
	if math.Abs(nBeam-want) > 1.0 {
		t.Errorf("beam = %.2f, want %.2f (length/aspect)", nBeam, want)
	}
}
