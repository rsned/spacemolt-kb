package hyperjump

import "testing"

func TestHeadingSweep(t *testing.T) {
	// Origin at 0; one system A due east at 1000 GU with a station.
	// A's corridor half-width is asin(100/1000)=~5.74deg, so integer headings
	// 0..5 and 355..359 land at A; everything else is void.
	o := System{ID: "O", Pos: Vec{0, 0}}
	a := System{ID: "A", Pos: Vec{1000, 0}, HasStation: true}
	sweep := HeadingSweep(o, []System{o, a}, 100)

	if len(sweep) != 3 {
		t.Fatalf("got %d ranges, want 3: %+v", len(sweep), sweep)
	}

	// Range 1: 0..5 -> A
	if sweep[0].StartDeg != 0 || sweep[0].EndDeg != 5 || sweep[0].LandsAt != "A" {
		t.Errorf("range[0] = %+v, want 0..5 -> A", sweep[0])
	}
	if !sweep[0].LandsAtStation {
		t.Errorf("range[0] should be flagged as a station landing")
	}
	if sweep[0].Ticks <= 0 || sweep[0].Distance <= 0 {
		t.Errorf("range[0] should have positive distance/ticks, got %+v", sweep[0])
	}
	// Range 2: 6..354 -> void
	if sweep[1].LandsAt != "" || sweep[1].StartDeg != 6 || sweep[1].EndDeg != 354 {
		t.Errorf("range[1] = %+v, want 6..354 void", sweep[1])
	}
	// Range 3: 355..359 -> A
	if sweep[2].StartDeg != 355 || sweep[2].EndDeg != 359 || sweep[2].LandsAt != "A" {
		t.Errorf("range[2] = %+v, want 355..359 -> A", sweep[2])
	}

	// Every whole degree 0..359 is covered exactly once.
	total := 0
	for _, r := range sweep {
		total += r.EndDeg - r.StartDeg + 1
	}
	if total != 360 {
		t.Errorf("ranges cover %d degrees, want 360", total)
	}
}

func TestHeadingSweep_picksClosest(t *testing.T) {
	// Two systems on the +X line: near(200) should win over far(1000) at heading 0.
	o := System{ID: "O", Pos: Vec{0, 0}}
	near := System{ID: "near", Pos: Vec{200, 0}}
	far := System{ID: "far", Pos: Vec{1000, 0}}
	sweep := HeadingSweep(o, []System{o, near, far}, 100)
	if sweep[0].StartDeg != 0 || sweep[0].LandsAt != "near" {
		t.Errorf("heading 0 should land at near, got %+v", sweep[0])
	}
}
