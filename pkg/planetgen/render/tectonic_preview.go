// Tectonic preview renderer (Phase 13 PoC).
// Generates an educational visualization of plates, crust, and boundaries
// with glowing colored lines indicating boundary type and activity.
//
// NOTE: This is proof-of-concept code for Phase 13 design. It references
// types (layers.Context, field.*Field) that will exist after Phase 12 merges.
// Stub definitions are provided below for documentation purposes.
//
// +build ignore
package render

import (
	"image/color"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

// Glow colors for boundary visualization.
// Alpha channel is pre-multiplied for easier blending.
var (
	// Convergent: red/orange glow (collision, mountains, trenches)
	convGlowInner = color.RGBA{255, 100, 50, 255}   // bright orange-red
	convGlowOuter = color.RGBA{180, 50, 20, 200}    // dimmer red

	// Divergent: blue/cyan glow (ridges, rifts)
	divGlowInner = color.RGBA{50, 200, 255, 255}    // bright cyan
	divGlowOuter = color.RGBA{20, 100, 180, 200}     // dimmer blue

	// Transform: yellow/green glow (faults)
	transGlowInner = color.RGBA{200, 255, 50, 255}   // bright yellow-green
	transGlowOuter = color.RGBA{120, 180, 30, 200}   // dimmer olive

	// Continental crust: green/brown base
	contLow  = color.RGBA{60, 80, 40, 255}   // dark green
	contHigh = color.RGBA{120, 100, 60, 255} // brownish

	// Oceanic crust: blue base
	oceanLow  = color.RGBA{20, 40, 80, 255}  // dark blue
	oceanHigh = color.RGBA{40, 60, 120, 255}  // medium blue
)

// RenderTectonicPreview generates the educational first-render visualization.
//
// Shows:
// - Heightmap-colored terrain (dark → light)
// - Plate boundaries as glowing colored lines
// - Continental vs oceanic crust distinction
// - Boundary intensity based on convergence/divergence magnitude
//
// Parameters:
// - ctx: Must have Plates, Crust, and TectonicFX populated
// - exaggeration: Height multiplier (0.5-5.0 typical) to make relief visible
// - glowRange: Distance in km for glow falloff (default 500)
func RenderTectonicPreview(ctx *layers.Context, exaggeration, glowRange float64) *cubemap.CubeMap {
	if ctx == nil || ctx.Plates == nil || ctx.Crust == nil || ctx.TectonicFX == nil {
		return nil
	}

	S := ctx.Size
	out := cubemap.New(S)

	// Pre-compute glow constants
	const defaultGlowRange = 500.0 // km
	if glowRange <= 0 {
		glowRange = defaultGlowRange
	}

	// Render each pixel
	for face := range out.Faces {
		for py := range S {
			rowStart := py * S
			for px := range S {
				i := rowStart + px

				// Get base height from crust
				baseH := ctx.Crust.BaseHeight.Faces[face][i]
				contFrac := ctx.Crust.ContinentalMask.Faces[face][i]

				// Apply exaggeration
				exH := baseH * exaggeration

				// Base color from height and crust type
				baseColor := heightmapCrustColor(exH, contFrac)

				// Overlay boundary glow
				glowColor := computeBoundaryGlow(ctx, face, i, glowRange)

				// Blend: base + glow
				finalColor := blendGlow(baseColor, glowColor)

				out.Set(face, px, py, finalColor)
			}
		}
	}

	return out
}

// heightmapCrustColor returns a color for the given height and continental fraction.
// Height is in [0,1] after exaggeration; continental fraction is in [0,1].
func heightmapCrustColor(h, contFrac float64) color.RGBA {
	// Clamp height
	if h < 0 {
		h = 0
	}
	if h > 1 {
		h = 1
	}

	// Interpolate between low and high based on height
	var low, high color.RGBA
	if contFrac > 0.5 {
		// Continental
		low, high = contLow, contHigh
	} else {
		// Oceanic
		low, high = oceanLow, oceanHigh
	}

	// Linear interpolation
	t := h
	return color.RGBA{
		R: uint8(float64(low.R)*(1-t) + float64(high.R)*t),
		G: uint8(float64(low.G)*(1-t) + float64(high.G)*t),
		B: uint8(float64(low.B)*(1-t) + float64(high.B)*t),
		A: 255,
	}
}

// computeBoundaryGlow returns the boundary glow color for a pixel.
// Samples convergent, divergent, and transform distances; applies glow
// to the nearest boundary with intensity modulated by magnitude.
func computeBoundaryGlow(ctx *layers.Context, face cubemap.Face, i int, glowRange float64) color.RGBA {
	pf := ctx.Plates
	fx := ctx.TectonicFX

	// Sample all boundary distances
	dConv := pf.Convergent[face][i]
	dDiv := pf.Divergent[face][i]
	dTrans := pf.Transform[face][i]

	// Find the nearest boundary type
	nearestDist := dConv
	nearestType := "conv"
	nearestMag := 0.5 // default magnitude

	if dDiv < nearestDist {
		nearestDist = dDiv
		nearestType = "div"
		nearestMag = fx.RidgeMag.Faces[face][i]
	}
	if dTrans < nearestDist {
		nearestDist = dTrans
		nearestType = "trans"
		nearestMag = 0.5 // transform has no meaningful magnitude
	}
	if nearestType == "conv" {
		// Use the largest convergent magnitude for this pixel
		// (belt > subd > arc contributions)
		if dConv < glowRange {
			// Determine which convergent type by checking distances in FX
			dBelt := fx.BeltDist.Faces[face][i]
			dSubd := fx.SubdDist.Faces[face][i]
			dArc := fx.ArcDist.Faces[face][i]

			if dBelt < dSubd && dBelt < dArc {
				nearestMag = fx.BeltMag.Faces[face][i]
			} else if dSubd < dArc {
				nearestMag = fx.SubdMag.Faces[face][i]
			} else {
				nearestMag = fx.ArcMag.Faces[face][i]
			}
		}
	}

	// If no boundary nearby, return transparent
	if nearestDist >= glowRange {
		return color.RGBA{0, 0, 0, 0}
	}

	// Compute intensity based on distance and magnitude
	// Intensity falls off with distance; magnitude boosts brightness
	distanceFalloff := 1.0 - nearestDist/glowRange
	magnitudeBoost := 0.5 + 0.5*nearestMag
	intensity := distanceFalloff * magnitudeBoost
	if intensity > 1 {
		intensity = 1
	}

	// Select glow color based on type
	var inner, outer color.RGBA
	switch nearestType {
	case "conv":
		inner, outer = convGlowInner, convGlowOuter
	case "div":
		inner, outer = divGlowInner, divGlowOuter
	case "trans":
		inner, outer = transGlowInner, transGlowOuter
	default:
		return color.RGBA{0, 0, 0, 0}
	}

	// Interpolate between outer (at edge of glow) and inner (at boundary)
	// Very close to boundary = inner, further out = outer
	t := intensity
	return color.RGBA{
		R: uint8(float64(outer.R)*(1-t) + float64(inner.R)*t),
		G: uint8(float64(outer.G)*(1-t) + float64(inner.G)*t),
		B: uint8(float64(outer.B)*(1-t) + float64(inner.B)*t),
		A: uint8(float64(outer.A)*(1-t) + float64(inner.A)*t),
	}
}

// blendGlow composites a glow color over a base color.
// Glow colors use alpha; base is opaque.
func blendGlow(base, glow color.RGBA) color.RGBA {
	if glow.A == 0 {
		return base
	}
	if glow.A == 255 {
		return glow
	}

	// Standard alpha blending: result = base * (1 - α) + glow * α
	alpha := float64(glow.A) / 255.0
	return color.RGBA{
		R: uint8(float64(base.R)*(1-alpha) + float64(glow.R)*alpha),
		G: uint8(float64(base.G)*(1-alpha) + float64(glow.G)*alpha),
		B: uint8(float64(base.B)*(1-alpha) + float64(glow.B)*alpha),
		A: 255,
	}
}

// RenderTectonicPreviewDebug is a test helper that returns a debug visualization
// showing only the plate boundaries without terrain coloring.
// Useful for verifying boundary classification.
func RenderTectonicPreviewDebug(ctx *layers.Context, S int) *cubemap.CubeMap {
	if ctx == nil || ctx.Plates == nil {
		return nil
	}

	out := cubemap.New(S)
	black := color.RGBA{0, 0, 0, 255}

	// Render each pixel
	for face := range out.Faces {
		for py := range S {
			rowStart := py * S
			for px := range S {
				i := rowStart + px

				pixelColor := black

				// Check boundary distances
				dConv := ctx.Plates.Convergent[face][i]
				dDiv := ctx.Plates.Divergent[face][i]
				dTrans := ctx.Plates.Transform[face][i]

				// Color code by nearest boundary
				minDist := dConv
				if dDiv < minDist {
					minDist = dDiv
				}
				if dTrans < minDist {
					minDist = dTrans
				}

				// Only color if within 200km of a boundary
				if minDist < 200 {
					if dConv == minDist {
						pixelColor = convGlowInner
					} else if dDiv == minDist {
						pixelColor = divGlowInner
					} else {
						pixelColor = transGlowInner
					}
				} else {
					// Show plate ID as grayscale (for debugging)
					plateID := ctx.Plates.PlateID[face][i]
					if plateID >= 0 {
						gray := uint8((plateID * 37) % 200) // deterministic but varied
						pixelColor = color.RGBA{gray, gray, gray, 255}
					}
				}

				out.Set(face, px, py, pixelColor)
			}
		}
	}

	return out
}

// =============================================================================
// STUB TYPE DEFINITIONS (for PoC documentation only)
// These will be replaced by actual imports from field and layers packages
// after Phase 12 merges to main.
// =============================================================================

// Context is a placeholder for layers.Context.
type Context struct {
	Plates     *PlateField
	Crust      *CrustField
	TectonicFX *TectonicFXField
}

// PlateField placeholder for field.PlateField.
type PlateField struct {
	Convergent [6][]float64
	Divergent  [6][]float64
	Transform  [6][]float64
	PlateID    [6][]int16
}

// CrustField placeholder for field.CrustField.
type CrustField struct {
	BaseHeight       *cubemap.CubeMapF
	ContinentalMask  *cubemap.CubeMapF
}

// TectonicFXField placeholder for field.TectonicFXField.
type TectonicFXField struct {
	BeltDist   *cubemap.CubeMapF
	BeltMag    *cubemap.CubeMapF
	SubdDist   *cubemap.CubeMapF
	SubdMag    *cubemap.CubeMapF
	ArcDist    *cubemap.CubeMapF
	ArcMag     *cubemap.CubeMapF
	RidgeDist  *cubemap.CubeMapF
	RidgeMag   *cubemap.CubeMapF
	RiftDist   *cubemap.CubeMapF
	RiftMag    *cubemap.CubeMapF
}
