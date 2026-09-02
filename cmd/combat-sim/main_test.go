package main

import (
	"strings"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/combatsim"
)

func testCal() *combatsim.Calibration {
	c := combatsim.DefaultCalibration()
	c.HitChanceByDistance = []float64{0.90, 0.80, 0.65, 0.50, 0.35, 0.22, 0.12}
	return c
}

func TestRunSwarmCLI(t *testing.T) {
	cat, err := combatsim.LoadCatalog("../../data/combat-sim/catalog")
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	if err := runSwarmCLI("prospect", "opus_magna", cat, testCal(), 25000, 60, 4000, 42, "", &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "prospect") || !strings.Contains(out, "opus_magna") {
		t.Fatalf("summary missing ids: %q", out)
	}
}
