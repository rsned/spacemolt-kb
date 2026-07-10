package render

import (
	"image/color"
	"math"

	planetcolor "github.com/rsned/spacemolt-kb/pkg/planetgen/color"
)

// SnowPixel blends white snow into c based on how far h is above
// snowLine and how close absLat is to the poles. Caller must guard
// h > snowLine before calling (the return value is only meaningful
// above the snow line). absLat is |asin(rawY)| / (pi/2).
//
// Extracted verbatim from the rocky.go Snow colorize stage so the
// flat-patch renderer can reuse the exact formula.
func SnowPixel(c color.RGBA, h, absLat, snowLine float64) color.RGBA {
	t := (h - snowLine) / (1.0 - snowLine)
	snowBlend := t * t * (3 - 2*t)
	snowBlend = math.Min(1.0, snowBlend*1.5)
	latBoost := 1.0 + absLat*0.5
	snowBlend = math.Min(1.0, snowBlend*latBoost)
	return planetcolor.BlendOkLab(c, whiteSnow, snowBlend*0.85)
}

// OceanPixel computes the shaded ocean (or lava, for lava_world
// profiles) color at a pixel. Caller must guard h < oceanLevel before
// calling. surfaceVar is the caller's already-sampled
// FractalNoise3D(warped dir, 4, 2.0, 0.5, 6.0) draw — it must always
// be consumed by the caller for rng stability even when this function
// isn't invoked (e.g. when the Ocean stage is bypassed).
//
// Extracted verbatim from the rocky.go Ocean colorize stage so the
// flat-patch renderer can reuse the exact formula.
func OceanPixel(oceanColor color.RGBA, planetType string, h, oceanLevel, surfaceVar float64) color.RGBA {
	depth := (oceanLevel - h) / oceanLevel
	var c color.RGBA
	if planetType == "lava_world" {
		brightness := 0.7 + depth*0.3
		if depth < 0.2 {
			brightness *= 0.6 + depth*2.0
		}
		brightness += (surfaceVar - 0.5) * 0.25
		brightness = math.Max(0.4, math.Min(1.2, brightness))
		lavaColor := planetcolor.Lerp(
			oceanColor,
			color.RGBA{R: 255, G: 160, B: 20, A: 255},
			surfaceVar*0.4,
		)
		c = planetcolor.Brighten(lavaColor, brightness)
	} else {
		shallowFactor := 1.0
		if depth < 0.15 {
			shallowFactor = 1.3 - depth*2.0
		}
		brightness := (1.0 - depth*0.5) * shallowFactor
		brightness += (surfaceVar - 0.5) * 0.15
		brightness = math.Max(0.5, math.Min(1.3, brightness))
		c = planetcolor.Brighten(oceanColor, brightness)
	}
	return c
}

// PolarCapPixel blends white ice into c when absLat crosses a
// noise-adjusted cap threshold. Caller must guard absLat > 1-capSize
// before calling; PolarCapPixel returns c unchanged when the
// noise-adjusted threshold isn't crossed (the noise draw can shrink
// the cap at its edge).
//
// Extracted verbatim from the rocky.go PolarCaps colorize stage so
// the flat-patch renderer can reuse the exact formula.
func PolarCapPixel(c color.RGBA, absLat, capSize, polarCapNoise, capEdgeNoise float64) color.RGBA {
	capThreshold := 1.0 - capSize
	noiseAmt := polarCapNoise
	if noiseAmt == 0 {
		noiseAmt = 0.08
	}
	adjustedThreshold := capThreshold + (capEdgeNoise-0.5)*noiseAmt
	if absLat > adjustedThreshold {
		blend := math.Min(1.0, (absLat-adjustedThreshold)*15)
		capColor := planetcolor.Brighten(whiteIce, 0.9+capEdgeNoise*0.2)
		return planetcolor.BlendOkLab(c, capColor, blend)
	}
	return c
}

// SlopeShadeSampled modulates the input color by a Lambertian dot
// product against a fixed sun direction. The surface normal is
// reconstructed from finite differences along two tangents orthogonal
// to the radial direction (rx, ry, rz), sampled via the caller-supplied
// sample closure (so callers can shade against a cube-map heightmap's
// seam-aware Sample or an arbitrary flat-patch height field).
//
// strength in [0,1]: 0 = unchanged; 1 = full diffuse modulation.
// exaggeration scales the gradient so subtle features still shade.
//
// Extracted verbatim from rocky.go's applySlopeShading, replacing the
// four hm.Sample(...) calls with the sample closure.
func SlopeShadeSampled(sample func(x, y, z float64) float64, c color.RGBA, rx, ry, rz, strength, exaggeration float64) color.RGBA {
	const eps = 0.005
	// Tangent basis orthogonal to (rx,ry,rz). Use world-up (0,1,0) unless
	// the radial is too close to it, in which case fall back to world-x.
	upx, upy, upz := 0.0, 1.0, 0.0
	if math.Abs(ry) > 0.95 {
		upx, upy, upz = 1.0, 0.0, 0.0
	}
	// t1 = normalize(cross(up, r))
	t1x := upy*rz - upz*ry
	t1y := upz*rx - upx*rz
	t1z := upx*ry - upy*rx
	t1n := math.Sqrt(t1x*t1x + t1y*t1y + t1z*t1z)
	if t1n == 0 {
		return c
	}
	t1x, t1y, t1z = t1x/t1n, t1y/t1n, t1z/t1n
	// t2 = cross(r, t1)
	t2x := ry*t1z - rz*t1y
	t2y := rz*t1x - rx*t1z
	t2z := rx*t1y - ry*t1x

	hu1 := sample(rx+eps*t1x, ry+eps*t1y, rz+eps*t1z)
	hu0 := sample(rx-eps*t1x, ry-eps*t1y, rz-eps*t1z)
	hv1 := sample(rx+eps*t2x, ry+eps*t2y, rz+eps*t2z)
	hv0 := sample(rx-eps*t2x, ry-eps*t2y, rz-eps*t2z)
	dHdu := (hu1 - hu0) / (2 * eps) * exaggeration
	dHdv := (hv1 - hv0) / (2 * eps) * exaggeration

	// Approximate world-space normal: in the (t1, t2, r) frame the
	// surface tangents are (1, 0, dHdu) and (0, 1, dHdv); their cross
	// product is (-dHdu, -dHdv, 1).  Express in world coords.
	nx := -dHdu*t1x - dHdv*t2x + rx
	ny := -dHdu*t1y - dHdv*t2y + ry
	nz := -dHdu*t1z - dHdv*t2z + rz
	nn := math.Sqrt(nx*nx + ny*ny + nz*nz)
	if nn == 0 {
		return c
	}
	nx, ny, nz = nx/nn, ny/nn, nz/nn

	// Fixed sun direction: upper-right-front, normalized.
	const lx, ly, lz = 0.6172, 0.6172, 0.4881 // (1, 1, 0.7)/|(1,1,0.7)|
	diff := nx*lx + ny*ly + nz*lz
	if diff < 0 {
		diff = 0
	}
	// Blend ambient and diffuse so flat areas read at neutral brightness.
	bright := (1.0 - strength) + strength*(0.4+0.8*diff)
	return planetcolor.Brighten(c, bright)
}
