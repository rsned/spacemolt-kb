package systemmap

import (
	"math"
	"strings"
	"testing"
)

func TestRadialLogMapping(t *testing.T) {
	pois := []POI{{Type: "sun"}, {Type: "planet", PositionX: 0.4}, {Type: "planet", PositionX: 30}}
	m := newRadial(pois, 250)
	if got := m.Px(30); math.Abs(got-250) > 1e-9 {
		t.Fatalf("outermost at %.2f px, want 250", got)
	}
	// Each power of ten past r0 takes about the same width.
	d1, d2 := m.Px(10)-m.Px(1), m.Px(30)-m.Px(3)
	if math.Abs(d1-d2) > 0.15*d1 {
		t.Errorf("decades unequal: 1→10 = %.1f px, 3→30 = %.1f px", d1, d2)
	}
	// The inner system gets room: 0.4 AU sits well away from the star.
	if m.Px(0.4) < 40 {
		t.Errorf("innermost body only %.1f px out", m.Px(0.4))
	}
	// Angles are kept.
	x, y := m.Point(0, 0, 3, 4)
	if math.Abs(math.Atan2(-y, x)-math.Atan2(4, 3)) > 1e-9 {
		t.Error("angle changed")
	}
}

func TestRadialStarOnly(t *testing.T) {
	m := newRadial([]POI{{Type: "sun"}}, 250)
	if m.Px(1) != 250 || m.ticks() != nil || m.rulerSVG(0, 0, 10) != "" {
		t.Error("a star-only system should fall back to linear with no ruler")
	}
}

func TestRulerCenteredAndMirrored(t *testing.T) {
	m := newRadial([]POI{{Type: "sun"}, {Type: "planet", PositionX: 0.4}, {Type: "planet", PositionX: 30}}, 250)
	svg := m.rulerSVG(400, 300, 580)
	for _, want := range []string{`>0</text>`, `>1</text>`, `>10</text>`, "log scale"} {
		if !strings.Contains(svg, want) {
			t.Errorf("ruler lacks %q", want)
		}
	}
	if strings.Count(svg, `>10</text>`) != 2 {
		t.Error("labels should mirror on both sides of the star")
	}
}

func TestLinearRadialHasRuler(t *testing.T) {
	m := newLinearRadial([]POI{{Type: "sun"}, {Type: "planet", PositionX: 0.4}, {Type: "planet", PositionX: 30}}, 250)
	if math.Abs(m.Px(15)-125) > 1e-9 {
		t.Fatalf("linear Px(15) = %v, want 125", m.Px(15))
	}
	svg := m.rulerSVG(400, 300, 580)
	if !strings.Contains(svg, "linear scale") || !strings.Contains(svg, `>10</text>`) {
		t.Errorf("linear ruler = %s", svg)
	}
}

func TestPrefixSVGIDs(t *testing.T) {
	in := `<defs><radialGradient id="sun-glow"/></defs><circle fill="url(#sun-glow)"/><use href="#ship-hauler"/><a href="../x/">x</a>`
	got := PrefixSVGIDs(in, "lin-")
	for _, want := range []string{`id="lin-sun-glow"`, `url(#lin-sun-glow)`, `href="#lin-ship-hauler"`, `href="../x/"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

func TestRenderSystemMapScaledDiffers(t *testing.T) {
	sys := &System{ID: "s", Name: "S", POIs: []POI{{ID: "st", Name: "Star", Type: "sun", Class: "G2V"},
		{ID: "a", Name: "A", Type: "planet", PositionX: 0.4}, {ID: "b", Name: "B", Type: "planet", PositionX: 30}}}
	log, lin := RenderSystemMap(sys, nil, false), RenderSystemMapScaled(sys, nil, false, true)
	if log == lin || !strings.Contains(log, "log scale") || !strings.Contains(lin, "linear scale") {
		t.Error("linear map should differ from the log map and say so on its ruler")
	}
}

// A linear ruler uses evenly spaced round steps, every tick labeled.
func TestLinearTicksEvenlySpaced(t *testing.T) {
	m := newLinearRadial([]POI{{Type: "sun"}, {Type: "planet", PositionX: 0.4}, {Type: "planet", PositionX: 31.7}}, 250)
	var vals []float64
	for _, tk := range m.ticks() {
		if !tk.major {
			t.Errorf("linear tick %v not labeled", tk.au)
		}
		vals = append(vals, tk.au)
	}
	want := []float64{5, 10, 15, 20, 25, 30}
	if len(vals) != len(want) {
		t.Fatalf("ticks = %v, want %v", vals, want)
	}
	for i := range want {
		if math.Abs(vals[i]-want[i]) > 1e-9 {
			t.Fatalf("ticks = %v, want %v", vals, want)
		}
	}
	if s := m.rulerSVG(400, 300, 580); !strings.Contains(s, `>15</text>`) {
		t.Error("intermediate linear ticks should be labeled")
	}
}

func TestNiceStep(t *testing.T) {
	for in, want := range map[float64]float64{31.7: 5, 0.115: 0.02, 83.4: 10, 4.6: 0.5, 1: 0.1} {
		if got := niceStep(in); math.Abs(got-want) > 1e-12 {
			t.Errorf("niceStep(%v) = %v, want %v", in, got, want)
		}
	}
}

// The log ruler runs on to the next power of ten when that is only a little
// past the outermost body (First Step: bodies at ~83 AU, ruler to 100).
func TestLogRulerExtendsToNextDecade(t *testing.T) {
	m := newRadial([]POI{{Type: "sun"}, {Type: "planet", PositionX: 83.3}, {Type: "planet", PositionX: 83.4}}, 250)
	if got := m.rulerEnd(); got != 100 {
		t.Fatalf("rulerEnd = %v, want 100", got)
	}
	if svg := m.rulerSVG(400, 300, 580); !strings.Contains(svg, `>100</text>`) {
		t.Error("ruler should label 100")
	}
	// A ruler never runs past the drawing area: with only a little room
	// beyond the outermost body, 12 AU does not stretch to 100.
	far := newRadial([]POI{{Type: "sun"}, {Type: "planet", PositionX: 0.01}, {Type: "planet", PositionX: 12}}, 250)
	far.rulerLimit = 260
	if got := far.rulerEnd(); got != 12 {
		t.Errorf("rulerEnd = %v, want 12 (100 is past the drawing area)", got)
	}
	// Linear rulers never extend.
	if lin := newLinearRadial([]POI{{Type: "sun"}, {Type: "planet", PositionX: 83.4}}, 250); lin.rulerEnd() != 83.4 {
		t.Errorf("linear rulerEnd = %v", lin.rulerEnd())
	}
}

func TestRenderSystemMapWithOptions(t *testing.T) {
	sys := &System{ID: "s", Name: "S", POIs: []POI{{ID: "st", Name: "Star", Type: "sun", Class: "G2V"},
		{ID: "a", Name: "A", Type: "planet", Class: "terran", PositionX: 1},
		{ID: "b", Name: "Belt", Type: "asteroid_belt", PositionX: 3}}}
	full := RenderSystemMapScaled(sys, nil, true, true)
	bare := RenderSystemMapWith(sys, nil, true, MapOptions{Linear: true, NoGates: true, NoLegend: true})
	if !strings.Contains(full, `stroke-dasharray="8,6"`) || strings.Contains(bare, `stroke-dasharray="8,6"`) {
		t.Error("NoGates should drop the gate ring")
	}
	if strings.Contains(bare, ">Asteroid Belt<") {
		t.Error("NoLegend should drop the legend")
	}
	same := RenderSystemMapWith(sys, nil, true, MapOptions{Linear: true, NoGates: true, NoLegend: true, FitAU: 30})
	if same == bare || !strings.Contains(same, ">30</text>") {
		t.Error("FitAU should rescale the map and extend the ruler to the fit distance")
	}
}
