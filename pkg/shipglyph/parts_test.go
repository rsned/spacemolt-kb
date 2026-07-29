package shipglyph

import (
	"math"
	"testing"
)

func TestPartHalfWidthOutsideSpanIsZero(t *testing.T) {
	p := HullPart{Kind: "box", Span: [2]float64{0.2, 0.8}, Half: 0.3}
	if got := partHalfWidth(p, 0.1); got != 0 {
		t.Errorf("before span = %v, want 0", got)
	}
	if got := partHalfWidth(p, 0.9); got != 0 {
		t.Errorf("after span = %v, want 0", got)
	}
	if got := partHalfWidth(p, 0.5); math.Abs(got-0.3) > 1e-9 {
		t.Errorf("inside span = %v, want 0.3", got)
	}
}

func TestPartHalfWidthBeamInterpolates(t *testing.T) {
	p := HullPart{
		Kind: "beam", Span: [2]float64{0, 1},
		Points: [][2]float64{{0, 0.0}, {0.5, 0.2}, {1, 0.0}},
	}
	mid := partHalfWidth(p, 0.5)
	if math.Abs(mid-0.2) > 1e-9 {
		t.Errorf("at control point = %v, want 0.2", mid)
	}
	quarter := partHalfWidth(p, 0.25)
	if quarter <= 0 || quarter >= 0.2 {
		t.Errorf("interpolated = %v, want strictly between 0 and 0.2", quarter)
	}
}

func TestBeamPartUsesAbsoluteSpineCoordinates(t *testing.T) {
	// Beam control points are authored in absolute spine coordinates: in
	// infer.go every beam's Points range equals its own Span. partHalfWidth
	// must therefore pass global t to beamHalfWidth, not part-local u.
	// Switching to u would pass every other test in this package while
	// silently deforming every multi-part hull.
	p := HullPart{
		Kind: "beam", Span: [2]float64{0.5, 1.0},
		Points: [][2]float64{{0.5, 0.10}, {1.0, 0.30}},
	}
	// t=0.75 is halfway between the control points, so the correct answer is
	// 0.20. Part-local u would be 0.5, which clamps to the first control
	// point and yields 0.10 instead.
	if got := partHalfWidth(p, 0.75); math.Abs(got-0.20) > 1e-9 {
		t.Errorf("partHalfWidth(t=0.75) = %v, want 0.20; 0.10 means part-local u is being passed", got)
	}
}

func TestPartHalfWidthEngineConeTapers(t *testing.T) {
	p := HullPart{Kind: "engine_cone", Span: [2]float64{0.8, 1.0}, Bells: 3}
	near, far := partHalfWidth(p, 0.82), partHalfWidth(p, 0.99)
	if near <= far {
		t.Errorf("cone should narrow toward the tail: %v then %v", near, far)
	}
}

func TestSampleProfileLengthAndNonNegativeWidth(t *testing.T) {
	d := Infer(Stats{ID: "war_wagon", Class: "Bulk Hauler", Faction: "crimson", Scale: 4})
	got := sampleProfile(d, StyleFor("crimson"), SeedOf("war_wagon"), 1)

	if len(got) != profileSamples {
		t.Fatalf("len = %d, want %d", len(got), profileSamples)
	}
	for i, p := range got {
		if p.Y < 0 {
			t.Fatalf("sample %d has negative Y %v; side +1 must stay non-negative", i, p.Y)
		}
		if p.X < 0 || p.X > 1 {
			t.Fatalf("sample %d has X %v outside [0,1]", i, p.X)
		}
	}
}

func TestSampleProfileIsDeterministic(t *testing.T) {
	d := Infer(Stats{ID: "yard_sale", Class: "Salvager", Faction: "outerrim", Scale: 3})
	st := StyleFor("outerrim")
	a := sampleProfile(d, st, SeedOf("yard_sale"), 1)
	b := sampleProfile(d, st, SeedOf("yard_sale"), 1)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("sample %d diverged: %v vs %v", i, a[i], b[i])
		}
	}
}

func TestSampleProfileSkewDiffersBySide(t *testing.T) {
	d := Infer(Stats{ID: "yard_sale", Class: "Salvager", Faction: "outerrim", Scale: 3})
	st := StyleFor("outerrim")
	star := sampleProfile(d, st, SeedOf("yard_sale"), 1)
	port := sampleProfile(d, st, SeedOf("yard_sale"), -1)

	same := true
	for i := range star {
		if math.Abs(star[i].Y) != math.Abs(port[i].Y) {
			same = false
			break
		}
	}
	if same {
		t.Errorf("outerrim sides are mirror-identical; skew should break symmetry")
	}
}

func TestOutlineCarriesNoSurfaceDetail(t *testing.T) {
	// A silhouette shows shape, not greeble. On a constant-width hull the
	// half-width may reverse direction only where a style deliberately shapes
	// it: at a panel joint, or at an extremum of the Voidborn lobe wave, which
	// runs 1.5 cycles and so turns at most three times. Everything above that
	// budget is surface detail. The old per-sample jitter reversed on roughly
	// half of all 96 samples.
	const lobeReversals = 3
	budget := skewPanels + lobeReversals
	d := Descriptor{Hull: []HullPart{{Kind: "box", Span: [2]float64{0, 1}, Half: 0.2}}}

	for _, faction := range []string{"crimson", "nebula", "solarian", "outerrim", "voidborn", "pirate", ""} {
		st := StyleFor(faction)
		prof := sampleProfile(d, st, SeedOf("straightedge"), 1)

		reversals, prevDir := 0, 0
		for i := 1; i < len(prof); i++ {
			dir := 0
			switch delta := prof[i].Y - prof[i-1].Y; {
			case delta > 1e-12:
				dir = 1
			case delta < -1e-12:
				dir = -1
			}
			if dir != 0 && prevDir != 0 && dir != prevDir {
				reversals++
			}
			if dir != 0 {
				prevDir = dir
			}
		}
		if reversals > budget {
			t.Errorf("%s: %d width reversals on a constant-width hull, want <= %d — "+
				"the profile is carrying surface detail", st.Name, reversals, budget)
		}
	}
}

func TestSampleProfileSymmetricFactionIsMirrored(t *testing.T) {
	d := Infer(Stats{ID: "comet", Class: "Liner", Faction: "nebula", Scale: 4})
	st := StyleFor("nebula")
	star := sampleProfile(d, st, SeedOf("comet"), 1)
	port := sampleProfile(d, st, SeedOf("comet"), -1)

	for i := range star {
		if math.Abs(star[i].Y-(-port[i].Y)) > 1e-9 {
			t.Fatalf("sample %d not mirrored: %v vs %v", i, star[i].Y, port[i].Y)
		}
	}
}
