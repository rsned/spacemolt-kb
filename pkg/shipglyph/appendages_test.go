package shipglyph

import (
	"strings"
	"testing"
)

func TestAppendageShapesBothSidesProduceTwoShapes(t *testing.T) {
	d := Descriptor{
		Aspect:     4,
		Hull:       []HullPart{{Kind: "box", Span: [2]float64{0, 1}, Half: 0.2}},
		Appendages: []Appendage{{Kind: "wing", At: 0.6, Sweep: 40, Span: 0.4, Side: "both"}},
	}
	got := AppendageShapes(d)

	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (one per side)", len(got))
	}
	if got[0].ID == got[1].ID {
		t.Errorf("both shapes share ID %q", got[0].ID)
	}
	var sawPos, sawNeg bool
	for _, sh := range got {
		for _, p := range sh.Poly {
			if p.Y > 1e-9 {
				sawPos = true
			}
			if p.Y < -1e-9 {
				sawNeg = true
			}
		}
	}
	if !sawPos || !sawNeg {
		t.Errorf("appendages do not appear on both sides")
	}
}

func TestAppendageShapesSingleSide(t *testing.T) {
	d := Descriptor{
		Hull:       []HullPart{{Kind: "box", Span: [2]float64{0, 1}, Half: 0.2}},
		Appendages: []Appendage{{Kind: "drone_rack", At: 0.5, Span: 0.2, Side: "port"}},
	}
	got := AppendageShapes(d)

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	for _, p := range got[0].Poly {
		if p.Y > 1e-9 {
			t.Errorf("port appendage has positive Y %v", p.Y)
		}
	}
}

func TestAppendageShapesExtendBeyondTheHull(t *testing.T) {
	d := Descriptor{
		Hull:       []HullPart{{Kind: "box", Span: [2]float64{0, 1}, Half: 0.2}},
		Appendages: []Appendage{{Kind: "wing", At: 0.5, Sweep: 30, Span: 0.5, Side: "both"}},
	}
	var maxY float64
	for _, sh := range AppendageShapes(d) {
		for _, p := range sh.Poly {
			if p.Y > maxY {
				maxY = p.Y
			}
		}
	}
	if maxY <= 0.2 {
		t.Errorf("max Y = %v, want beyond the 0.2 hull half-width", maxY)
	}
}

func TestAppendageShapesIDsAreStableAndUnique(t *testing.T) {
	d := Infer(Stats{ID: "comet", Class: "Liner", Faction: "nebula", Scale: 4})
	seen := map[string]bool{}
	for _, sh := range AppendageShapes(d) {
		if seen[sh.ID] {
			t.Errorf("duplicate appendage ID %q", sh.ID)
		}
		seen[sh.ID] = true
	}
	if len(seen) == 0 {
		t.Errorf("a Nebula liner should have wings")
	}
}

func TestAppendageShapesNoneIsEmpty(t *testing.T) {
	d := Descriptor{Hull: []HullPart{{Kind: "box", Span: [2]float64{0, 1}, Half: 0.2}}}
	if got := AppendageShapes(d); len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestAppendageShapesSanitizesKindForMarkup(t *testing.T) {
	d := Descriptor{
		Hull:       []HullPart{{Kind: "box", Span: [2]float64{0, 1}, Half: 0.2}},
		Appendages: []Appendage{{Kind: `wing" onload="x`, At: 0.5, Span: 0.2, Side: "port"}},
	}
	got := AppendageShapes(d)

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if strings.ContainsAny(got[0].ID, `"<>&`) {
		t.Errorf("ID %q still contains markup-breaking characters", got[0].ID)
	}
	if strings.ContainsAny(got[0].Kind, `"<>&`) {
		t.Errorf("Kind %q still contains markup-breaking characters", got[0].Kind)
	}
}
