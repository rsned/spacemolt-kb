package biome

import (
	"math"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// RainShadowField holds a per-pixel multiplier for the humidity (M)
// input to the Whittaker biome lookup. Values >1 boost moisture
// (windward / orographic uplift); values <1 dry the air (lee shadow).
// Multiplier is sized to match the source heightmap (Size×Size per
// face).
type RainShadowField struct {
	Size       int
	Multiplier [cubemap.NumFaces][]float64
}

// GenerateRainShadow computes per-pixel orographic moisture multiplier
// from a heightmap. Returns nil when cfg.WalkSteps == 0 (disabled).
//
// At each pixel the algorithm:
//  1. determines the prevailing wind tangent at the pixel direction;
//     the returned vector points *toward where the air came from*
//     (i.e. for tropical easterlies it points east);
//  2. walks the great-circle from that pixel in that direction (so
//     the walk traces back upwind, toward the source of the air) for
//     cfg.WalkSteps steps of cfg.StepArcRad each, sampling the
//     heightmap;
//  3. emits a multiplier based on whether a peak above
//     cfg.MountainCutoff was crossed during the walk:
//     - walk hits a peak immediately (peak is upwind of us → we are
//     in its lee):                                cfg.LeeFactor
//     - walk passes the peak and continues without further peaks
//     (we are upwind of the ridge it crossed; the air arriving
//     here was lifted somewhere far away — no orographic effect
//     local to us):                               1.0
//     - walk only finds peaks late in the trace (the air is climbing
//     into a ridge directly upstream of us, ie. we are on the
//     windward face): we approximate this by emitting the boost
//     when the *first* hit is the very first step of the walk,
//     otherwise treating later-step hits as the lee case.
//
// Because the walk discretizes the great-circle, simple "upwind /
// lee" detection works as follows: a peak found at any walk step
// means "upwind of us is a mountain"; if we then keep walking and
// the next step is no longer a peak we are clearly past the ridge,
// confirming we are in the lee. If the very first sampled step is a
// peak, we treat the current pixel as the windward face of the
// ridge (orographic uplift → boost).
//
// The heightmap is expected in the [0,1] normalized range that
// follows the rocky pipeline's Normalize stage; cfg.MountainCutoff is
// interpreted as a value in that space (e.g. 0.65 means "65th
// percentile of the normalized height range counts as a mountain
// peak").
func GenerateRainShadow(heightmap *cubemap.CubeMapF, cfg types.RainShadowConfig) *RainShadowField {
	if cfg.WalkSteps <= 0 || heightmap == nil {
		return nil
	}
	S := heightmap.Size
	out := &RainShadowField{Size: S}
	for face := range out.Multiplier {
		out.Multiplier[face] = make([]float64, S*S)
	}
	for face := range cubemap.Face(cubemap.NumFaces) {
		for py := range S {
			for px := range S {
				dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
				wind := prevailingWindTangent(dx, dy, dz)
				m := walkUpwindForOrography(heightmap, dx, dy, dz, wind, cfg)
				out.Multiplier[face][py*S+px] = m
			}
		}
	}
	return out
}

// prevailingWindTangent returns the unit tangent vector of the
// prevailing wind at sphere direction (dx, dy, dz).
//
// Three-cell model with Earth-like Coriolis:
//   - tropics (|lat| < 30°):    easterlies (return +east tangent)
//   - midlatitudes (30°–60°):   westerlies (return -east = +west tangent)
//   - polar (|lat| > 60°):      polar easterlies (return +east tangent)
//
// "East" in the tangent plane is computed as cross(north, dir) with
// north = (0, 1, 0); for our coordinate frame this evaluates to the
// closed-form (-z, 0, x). The vector is normalized; at the poles
// (where the formula degenerates) we fall back to (1, 0, 0).
func prevailingWindTangent(dx, dy, dz float64) [3]float64 {
	// Clamp dy to valid asin domain to guard against floating drift.
	sinLat := dy
	if sinLat > 1 {
		sinLat = 1
	} else if sinLat < -1 {
		sinLat = -1
	}
	absLat := math.Abs(math.Asin(sinLat))
	const tropicLimit = math.Pi / 6.0           // 30°
	const polarLimit = math.Pi * 60.0 / 180.0   // 60°
	eastward := absLat < tropicLimit || absLat > polarLimit

	// East tangent = cross((0,1,0), (dx,dy,dz)) = (-dz, 0, dx).
	ex, ey, ez := -dz, 0.0, dx
	n := math.Sqrt(ex*ex + ey*ey + ez*ez)
	if n < 1e-9 {
		// At the poles the tangent direction is degenerate; the rain
		// shadow concept doesn't really apply. Pick a stable fallback
		// so the returned vector is always unit-length and finite.
		return [3]float64{1, 0, 0}
	}
	ex, ey, ez = ex/n, ey/n, ez/n
	if !eastward {
		ex, ey, ez = -ex, -ey, -ez
	}
	return [3]float64{ex, ey, ez}
}

// walkUpwindForOrography walks the great-circle from (dx,dy,dz) in
// the wind direction for cfg.WalkSteps steps of cfg.StepArcRad each.
// At each step the heightmap is sampled (bilinearly) at the new
// direction; we track whether a peak above cfg.MountainCutoff has
// been crossed during the walk.
//
// The wind tangent is held constant for the duration of the walk.
// Parallel transport over the short arcs used here (typically <60°
// total) is close enough to the identity that the simpler "constant
// wind" approximation produces stable rain shadows without
// introducing the trigonometric machinery that proper transport
// would require.
//
// Returned multiplier:
//   - walk encounters a peak before any non-peak step: 1 + WindRainBoost
//   - walk passes a peak then continues past it:       LeeFactor
//   - otherwise:                                       1.0
func walkUpwindForOrography(heightmap *cubemap.CubeMapF, dx, dy, dz float64, wind [3]float64, cfg types.RainShadowConfig) float64 {
	cosA := math.Cos(cfg.StepArcRad)
	sinA := math.Sin(cfg.StepArcRad)

	// Re-orthogonalize the wind to the position so the great-circle
	// formula stays unit-length under accumulation. This is an exact
	// projection: w := w - (w·d) d, normalize.
	dot := wind[0]*dx + wind[1]*dy + wind[2]*dz
	wx := wind[0] - dot*dx
	wy := wind[1] - dot*dy
	wz := wind[2] - dot*dz
	wn := math.Sqrt(wx*wx + wy*wy + wz*wz)
	if wn < 1e-9 {
		return 1.0
	}
	wx, wy, wz = wx/wn, wy/wn, wz/wn

	// Walk along the great-circle. After each step, advance the
	// position by stepArc *and* re-derive a fresh perpendicular wind
	// vector that stays in the tangent plane at the new position
	// (still using the same global wind direction as a target).
	posX, posY, posZ := dx, dy, dz
	peakSeen := false
	currentMul := 1.0
	for range cfg.WalkSteps {
		nx := cosA*posX + sinA*wx
		ny := cosA*posY + sinA*wy
		nz := cosA*posZ + sinA*wz
		// Renormalize to absorb floating-point drift.
		nn := math.Sqrt(nx*nx + ny*ny + nz*nz)
		if nn < 1e-12 || math.IsNaN(nn) {
			break
		}
		nx, ny, nz = nx/nn, ny/nn, nz/nn

		h := heightmap.Sample(nx, ny, nz)
		isPeak := h >= cfg.MountainCutoff
		switch {
		case isPeak && !peakSeen:
			// First peak in the upwind direction → orographic uplift,
			// rain falls here. Tag and keep walking; if the walk
			// terminates without encountering another peak the result
			// is the boost.
			peakSeen = true
			currentMul = 1.0 + cfg.WindRainBoost
		case !isPeak && peakSeen:
			// We've crossed past the peak — we're in its lee.
			currentMul = cfg.LeeFactor
		}

		// Advance position; re-orthogonalize the wind tangent at the
		// new position to keep it in the tangent plane.
		posX, posY, posZ = nx, ny, nz
		wDot := wx*posX + wy*posY + wz*posZ
		wx -= wDot * posX
		wy -= wDot * posY
		wz -= wDot * posZ
		wn2 := math.Sqrt(wx*wx + wy*wy + wz*wz)
		if wn2 < 1e-9 {
			break
		}
		wx, wy, wz = wx/wn2, wy/wn2, wz/wn2
	}

	// Clamp to a sensible band so callers can rely on the multiplier
	// staying in (0, 2]. The lee factor should always be a positive
	// multiplier (never 0), and the boost should not go absurdly high.
	if currentMul <= 0 {
		currentMul = 1e-3
	}
	if currentMul > 2 {
		currentMul = 2
	}
	return currentMul
}
