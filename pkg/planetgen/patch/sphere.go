package patch

import (
	"fmt"

	planetcolor "github.com/rsned/spacemolt-kb/pkg/planetgen/color"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/field"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// canonicalFace is the production face size droplet counts are
// calibrated against (planetgen.DefaultFaceSize; kept as a local const
// to avoid importing the root package).
const canonicalFace = 1024

// SphereData is everything the patch needs from the sphere-global
// tectonic precompute at face size STect.
type SphereData struct {
	STect   int
	Master  int64
	Profile *types.PlanetProfile

	Jitter *noise.JitterField
	Plates *field.PlateField
	Crust  *field.CrustField
	FX     *field.TectonicFXField

	// Normalize affine measured on the S_tect heightmap after
	// smooth (production normalizes with the global min/max at
	// S_prod; this is the documented S_tect approximation).
	HMin, HMax float64
	// SeaLevel0 is the pre-flow quantile (drives coastal +
	// erosion, exactly as production uses its first-derived
	// level); SeaLevel is the post-flow re-quantile (drives
	// waterline coloring).
	SeaLevel0, SeaLevel float64
}

// ComputeSphere runs the sphere-global part of the crust-path rocky
// pipeline at face size sTect. It mirrors render/rocky.go stage order:
// crust init → Detail/PeaksValleys splines (Continentalness skipped on
// the crust path) → TectonicFX → smooth → normalize → quantile sea
// level → erode → flow+carve → re-quantile.
func ComputeSphere(profile *types.PlanetProfile, master int64, sTect int) (*SphereData, error) {
	if profile == nil || profile.Crust.MajorPlates <= 0 {
		return nil, fmt.Errorf("patch: Patch Lab requires a crust-enabled profile (crust.majorPlates > 0)")
	}
	if profile.Provinces.Count > 0 {
		return nil, fmt.Errorf("patch: Patch Lab doesn't support province modulation yet (profile.Provinces.Count > 0)")
	}
	reportProgress("sphere:jitter", 1, 10)
	jitter := noise.GenerateJitter(profile, master, sTect)
	reportProgress("sphere:plates", 2, 10)
	plates := field.GeneratePlates(profile, master, sTect)
	if plates == nil {
		return nil, fmt.Errorf("patch: plate generation returned nil")
	}
	reportProgress("sphere:crust", 3, 10)
	crust := field.GenerateCrust(profile, master, sTect, plates)
	reportProgress("sphere:fx", 4, 10)
	fx := field.ClassifyTectonics(plates, crust, profile.RadiusKm)

	// Heightmap init from the crust raft.
	hm := crust.BaseHeight.Clone()

	// Control-field splines: crust path skips index 0
	// (Continentalness); indices 1 (Detail) and 2 (PeaksValleys)
	// contribute. Mirrors rocky.go's fast path.
	reportProgress("sphere:splines", 5, 10)
	fields := field.GenerateControlFields(master, profile.ControlConfig, sTect, jitter)
	cf := [3]types.ControlField{profile.ControlConfig.Continentalness, profile.ControlConfig.Detail, profile.ControlConfig.PeaksValleys}
	for face := range cubemap.Face(cubemap.NumFaces) {
		for i := range hm.Faces[face] {
			h := hm.Faces[face][i]
			for k := 1; k < 3; k++ {
				h += planetcolor.EvalSpline(cf[k].Spline, fields[k].Faces[face][i])
			}
			hm.Faces[face][i] = h
		}
	}

	reportProgress("sphere:tectonic-fx", 6, 10)
	field.ApplyTectonicFX(hm, fx, crust, plates, profile.TectonicFX, master, sTect)
	if profile.HeightSmoothRadius > 0 {
		reportProgress("sphere:smooth", 7, 10)
		hm = field.SmoothHeightmap(hm, profile.HeightSmoothRadius, sTect)
	}

	// Normalize: global min/max rescale (inline in rocky.go too).
	reportProgress("sphere:normalize", 8, 10)
	hMin, hMax := hm.Faces[0][0], hm.Faces[0][0]
	for face := range cubemap.Face(cubemap.NumFaces) {
		for _, v := range hm.Faces[face] {
			if v < hMin {
				hMin = v
			}
			if v > hMax {
				hMax = v
			}
		}
	}
	if hMax > hMin {
		inv := 1 / (hMax - hMin)
		for face := range cubemap.Face(cubemap.NumFaces) {
			for i, v := range hm.Faces[face] {
				hm.Faces[face][i] = (v - hMin) * inv
			}
		}
	}

	seaLevel0 := field.QuantileSeaLevel(hm, 1-crust.LandFraction)

	// Erode + flow at S_tect purely to re-derive the post-flow sea
	// level the way production does. Droplet count auto-scales by
	// face area with a 5000 floor, exactly mirroring rocky.go's
	// scaling block (L738-745): float-scaled by area ratio against
	// the canonical S=1024 face, truncated to int, then floored to
	// 5000 ONLY when the profile's unscaled Droplets was already
	// >= 5000 (so profiles with a small Droplets count are not
	// forced up to the floor).
	ecfg := profile.Erosion
	if ecfg.Droplets > 0 {
		reportProgress("sphere:erode", 9, 10)
		scale := float64(sTect*sTect) / float64(canonicalFace*canonicalFace)
		n := int(float64(ecfg.Droplets) * scale)
		if n < 5000 && ecfg.Droplets >= 5000 {
			n = 5000
		}
		ecfg.Droplets = n
		hm = field.Erode(master, hm, ecfg, seaLevel0, sTect)
	}
	reportProgress("sphere:flow", 10, 10)
	if ff := field.GenerateFlow(hm, profile.Flow); ff != nil {
		field.CarveRivers(hm, ff, profile.Flow)
	}
	seaLevel := field.QuantileSeaLevel(hm, 1-crust.LandFraction)

	return &SphereData{
		STect: sTect, Master: master, Profile: profile,
		Jitter: jitter, Plates: plates, Crust: crust, FX: fx,
		HMin: hMin, HMax: hMax,
		SeaLevel0: seaLevel0, SeaLevel: seaLevel,
	}, nil
}
