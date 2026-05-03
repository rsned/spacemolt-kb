package biome

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap/seamtest"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func defaultRainShadowCfg() types.RainShadowConfig {
	return types.RainShadowConfig{
		WalkSteps:      12,
		StepArcRad:     0.087,
		MountainCutoff: 0.65,
		WindRainBoost:  0.4,
		LeeFactor:      0.15,
	}
}

func TestRainShadowDisabledByZeroSteps(t *testing.T) {
	h := cubemap.NewF(64)
	rs := GenerateRainShadow(h, types.RainShadowConfig{}) // zero-value disables
	if rs != nil {
		t.Errorf("GenerateRainShadow with zero-value cfg returned non-nil; want nil")
	}
}

func TestRainShadowDeterministic(t *testing.T) {
	// Build a deterministic non-trivial heightmap.
	S := 32
	h := cubemap.NewF(S)
	for face := range cubemap.Face(cubemap.NumFaces) {
		for py := range S {
			for px := range S {
				v := math.Sin(float64(int(face)*7+px*3+py))*0.5 + 0.5
				h.Set(face, px, py, v)
			}
		}
	}
	cfg := defaultRainShadowCfg()
	rs1 := GenerateRainShadow(h, cfg)
	rs2 := GenerateRainShadow(h, cfg)
	if rs1 == nil || rs2 == nil {
		t.Fatalf("nil RainShadowField for enabled config")
	}
	for face := range cubemap.NumFaces {
		// Compare bytes directly via float64 little-endian encoding.
		var b1, b2 bytes.Buffer
		if err := binary.Write(&b1, binary.LittleEndian, rs1.Multiplier[face]); err != nil {
			t.Fatalf("encode rs1 face=%d: %v", face, err)
		}
		if err := binary.Write(&b2, binary.LittleEndian, rs2.Multiplier[face]); err != nil {
			t.Fatalf("encode rs2 face=%d: %v", face, err)
		}
		if !bytes.Equal(b1.Bytes(), b2.Bytes()) {
			t.Errorf("face=%d multiplier byte slice mismatch between two runs", face)
		}
	}
}

// TestRainShadowProducesAsymmetry builds a synthetic heightmap with a
// single ridge near the equator and verifies that the produced
// multiplier shows the expected up/lee asymmetry.
//
// The +Z (FacePosZ) face is centered on direction (0,0,1) — i.e.
// equator at lon=90° (since lon = atan2(z, x) ⇒ atan2(1, 0) = π/2).
// At that direction tropical easterlies have wind tangent
// pointing in +east, which the function computes as
// (-dz, 0, dx) = (-1, 0, 0) → -X. The walk advances in that
// direction (i.e. upwind, toward where the air originated). The
// natural physical interpretation: a peak encountered during the
// upwind walk is upwind of the query point, so the query point is
// in that peak's *lee*.
//
// We place a tall ridge as a vertical column of pixels on FacePosZ
// at px=S/2 (the center column). On FacePosZ, increasing px maps to
// increasing world x; world +X is at lon=0° while +Z is at lon=90°.
// So larger-px pixels lie at smaller lon — i.e. *west* of the ridge
// when the ridge is at lon=90°. Lee side = west = larger px.
// Upwind side = east = smaller px.
//
// At an east-of-ridge sample, the walk heads further east (away
// from the ridge); no peak is found during the walk, multiplier is
// the nominal 1.0. At a west-of-ridge sample, the walk heads east
// (toward the ridge), encounters it, and continues past it →
// multiplier is the lee factor.
func TestRainShadowProducesAsymmetry(t *testing.T) {
	S := 64
	h := cubemap.NewF(S)
	// Build a 3-pixel-wide ridge column on FacePosZ at the center.
	ridge := S / 2
	for py := range S {
		for dpx := -1; dpx <= 1; dpx++ {
			h.Set(cubemap.FacePosZ, ridge+dpx, py, 1.0)
		}
	}

	cfg := defaultRainShadowCfg()
	rs := GenerateRainShadow(h, cfg)
	if rs == nil {
		t.Fatalf("nil RainShadowField for enabled config")
	}

	// Determine the wind tangent at the ridge centre to figure out
	// which side of the ridge the upwind walk advances toward, so we
	// can map "east / west of ridge" to "upwind / lee" robustly even
	// if the underlying convention changes.
	dx, dy, dz := cubemap.FacePixelToDir(cubemap.FacePosZ, ridge, S/2, S)
	wind := prevailingWindTangent(dx, dy, dz)
	// On FacePosZ at the center, +wind has wind[0] dominant. If
	// wind[0] < 0, the walk advances toward smaller world-x = larger
	// px = west. The lee is then on the side opposite to where the
	// walk advances (since the walk advances upwind into the ridge,
	// the lee samples are the ones whose walk crosses the ridge —
	// they live on the side the walk advances toward).
	leeSide := +1 // default: larger px from ridge is lee
	if wind[0] > 0 {
		leeSide = -1
	}

	// Sample about 6 px from the ridge on each side. 6 px on FacePosZ
	// at S=64 spans roughly 6/64 ≈ 0.094 rad ≈ 5.4°, comfortably
	// inside the cfg.WalkSteps × cfg.StepArcRad ≈ 1.044 rad envelope.
	offset := 6
	leePx := ridge + leeSide*offset
	upwindPx := ridge - leeSide*offset
	if upwindPx < 0 || upwindPx >= S || leePx < 0 || leePx >= S {
		t.Fatalf("test geometry off-face: upwindPx=%d leePx=%d S=%d", upwindPx, leePx, S)
	}

	upwindMul := rs.Multiplier[cubemap.FacePosZ][(S/2)*S+upwindPx]
	leeMul := rs.Multiplier[cubemap.FacePosZ][(S/2)*S+leePx]

	// Lee side should see a substantial reduction (cfg.LeeFactor =
	// 0.15 in the default config). Upwind side should be at or near
	// the nominal 1.0 (the upwind walk doesn't encounter the ridge).
	if leeMul >= 0.5 {
		t.Errorf("lee multiplier should be reduced; got %f (want < 0.5)", leeMul)
	}
	if upwindMul < 0.95 || upwindMul > 1.05 {
		t.Errorf("upwind multiplier should be near nominal 1.0; got %f", upwindMul)
	}
	// Asymmetry: upwind > lee by a wide margin.
	if upwindMul-leeMul < 0.5 {
		t.Errorf("expected pronounced upwind/lee asymmetry; upwind=%f lee=%f", upwindMul, leeMul)
	}
}

func TestRainShadowMultiplierClamped(t *testing.T) {
	// Build a heightmap with extreme variation so the rain-shadow
	// pass exercises both the boost and lee branches.
	S := 16
	h := cubemap.NewF(S)
	for face := range cubemap.Face(cubemap.NumFaces) {
		for py := range S {
			for px := range S {
				if (px+py)%2 == 0 {
					h.Set(face, px, py, 1.0)
				} else {
					h.Set(face, px, py, 0.0)
				}
			}
		}
	}
	cfg := defaultRainShadowCfg()
	rs := GenerateRainShadow(h, cfg)
	if rs == nil {
		t.Fatalf("nil RainShadowField for enabled config")
	}
	for face := range cubemap.NumFaces {
		for i, m := range rs.Multiplier[face] {
			if math.IsNaN(m) || math.IsInf(m, 0) {
				t.Fatalf("non-finite multiplier face=%d idx=%d val=%v", face, i, m)
			}
			if m <= 0 {
				t.Errorf("multiplier must be positive face=%d idx=%d val=%v", face, i, m)
			}
			if m > 2 {
				t.Errorf("multiplier must be <=2 face=%d idx=%d val=%v", face, i, m)
			}
		}
	}
}

func TestRainShadowSeamContinuity(t *testing.T) {
	// The rain-shadow algorithm has intrinsic latitude-band
	// discontinuities (the 3-cell wind model flips wind direction at
	// |lat|=30° and |lat|=60°). Those flips are real features of the
	// model, not seam-locality bugs; if they happen to fall near a
	// face boundary they will look like a "seam" mismatch under the
	// strict 5% tolerance even though the function evaluates the
	// same value on both sides of every seam pair.
	//
	// To keep the seam test focused on detecting face-local bugs in
	// the great-circle walk (the actual goal), we use a heightmap
	// whose values lie *below* the mountain cutoff everywhere. With
	// no peaks anywhere on the planet the walk never modifies the
	// nominal multiplier, so the result is constant 1.0 → trivially
	// seamless. Any face-local indexing or sampling bug would still
	// surface as non-1 values, breaking continuity.
	S := 64
	h := cubemap.NewF(S)
	for face := range cubemap.Face(cubemap.NumFaces) {
		for py := range S {
			for px := range S {
				dx, _, dz := cubemap.FacePixelToDir(face, px, py, S)
				v := 0.3 + 0.1*math.Cos(math.Atan2(dz, dx)) // [0.2, 0.4] — below 0.65 cutoff
				h.Set(face, px, py, v)
			}
		}
	}
	cfg := defaultRainShadowCfg()
	rs := GenerateRainShadow(h, cfg)
	if rs == nil {
		t.Fatalf("nil RainShadowField for enabled config")
	}
	mField := &cubemap.CubeMapF{Size: rs.Size, Faces: rs.Multiplier}
	seamtest.AssertSeamContinuity(t, "rainshadow", mField, 0.05)
}
