package feature

import (
	"image/color"
	"math"
)

// ApplyEjecta brightens base near recent, large craters with an angular
// ray pattern that fades with age² and with great-circle distance from
// the rim. Returns base unchanged when no craters touch the pixel.
//
// dx, dy, dz must be a unit-sphere direction in the same convention as
// crater (lat, lon) — i.e. (cos(lat)cos(lon), sin(lat), cos(lat)sin(lon)).
func ApplyEjecta(base color.RGBA, dx, dy, dz float64, craters []Crater) color.RGBA {
	const (
		raysPerCrater   = 8
		outerRimMul     = 6.0  // ejecta extends to 6× the crater radius
		strengthPerHit  = 0.4  // accumulator scale
		brightAddPerOne = 60.0 // RGB units added at full brightness
		minRadius       = 0.01 // skip small bowls — too tight for visible rays
		minAge          = 0.3  // older than this fades out
	)
	bright := 0.0
	for _, cr := range craters {
		if cr.IsSecondary || cr.Age < minAge || cr.Radius < minRadius {
			continue
		}
		cx := math.Cos(cr.Lat) * math.Cos(cr.Lon)
		cy := math.Sin(cr.Lat)
		cz := math.Cos(cr.Lat) * math.Sin(cr.Lon)
		dot := dx*cx + dy*cy + dz*cz
		if dot <= 0 {
			continue
		}
		if dot > 1 {
			dot = 1
		}
		ang := math.Acos(dot)
		rOuter := cr.Radius * outerRimMul
		if ang > rOuter || ang < cr.Radius {
			continue
		}
		// Radial falloff from rim outward, smoothstep-shaped.
		t := 1 - (ang-cr.Radius)/(rOuter-cr.Radius)
		t *= t

		// Azimuth around the crater: project (d - dot·c) onto the local
		// tangent frame at (lat, lon). North tangent and east tangent are
		// the spherical basis vectors. atan2(east, north) gives a stable
		// 0..2π bearing that's anchored to the crater, not the global frame.
		px := dx - dot*cx
		py := dy - dot*cy
		pz := dz - dot*cz
		sinLat := math.Sin(cr.Lat)
		cosLat := math.Cos(cr.Lat)
		sinLon := math.Sin(cr.Lon)
		cosLon := math.Cos(cr.Lon)
		// north tangent: derivative of (cos(lat)cos(lon), sin(lat), cos(lat)sin(lon)) wrt lat
		nx, ny, nz := -sinLat*cosLon, cosLat, -sinLat*sinLon
		// east tangent: derivative wrt lon, normalised by cos(lat)
		ex, ez := -sinLon, cosLon
		azN := px*nx + py*ny + pz*nz
		azE := px*ex + pz*ez
		theta := math.Atan2(azE, azN)
		ray := 0.5 + 0.5*math.Cos(raysPerCrater*theta)

		bright += t * ray * cr.Age * cr.Age * strengthPerHit
	}
	if bright <= 0 {
		return base
	}
	if bright > 1 {
		bright = 1
	}
	add := bright * brightAddPerOne
	r := uint16(float64(base.R) + add)
	g := uint16(float64(base.G) + add)
	b := uint16(float64(base.B) + add)
	if r > 255 {
		r = 255
	}
	if g > 255 {
		g = 255
	}
	if b > 255 {
		b = 255
	}
	return color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: base.A}
}
