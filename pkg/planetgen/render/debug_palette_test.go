package render

import (
	"testing"

	planetcolor "github.com/rsned/spacemolt-kb/pkg/planetgen/color"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

func TestClassifyInputBandsBoundaries(t *testing.T) {
	sp := planetcolor.Spline{Knots: []planetcolor.SplineKnot{
		{Input: 0, Output: 0},
		{Input: 0.5, Output: 0.3},
		{Input: 1, Output: 0.6},
	}}
	field := cubemap.NewF(4)
	field.Set(cubemap.FacePosX, 0, 0, 0.25) // band 0
	field.Set(cubemap.FacePosX, 1, 0, 0.75) // band 1
	out := ClassifySplineInputBands(field, sp, 4)
	if out.Get(cubemap.FacePosX, 0, 0) == out.Get(cubemap.FacePosX, 1, 0) {
		t.Error("two bands should differ")
	}
}

func TestSignedToRGBARed(t *testing.T) {
	field := cubemap.NewF(4)
	field.Set(cubemap.FacePosX, 0, 0, -0.5)
	out := SignedToRGBA(field, 4, 1)
	px := out.Get(cubemap.FacePosX, 0, 0)
	if px.R == 0 {
		t.Error("negative pixel should produce red component")
	}
}
