package noise

// CoastalGen wraps a noise.Generator at a fixed seed for coastal-noise
// sampling. Construct once per planet to keep the noise stream stable.
type CoastalGen struct{ g *Generator }

// NewCoastalGen constructs a new CoastalGen with the given seed.
func NewCoastalGen(seed int64) *CoastalGen {
	return &CoastalGen{g: New(seed)}
}

// ApplyCoastal applies the coastal enhancement formula at one pixel.
// dx,dy,dz is the unit-sphere direction; height is the current height
// in [0,1]; distToCoast is in [0,1] (0 = on coast, 1 = far inland or
// far offshore); amp is the master strength (0 disables); threshold
// is the distance-to-coast cutoff; freq is the base frequency.
// The formula is: e_coast = e + α·(1 − e⁴)·(n4 + n5/2 + n6/4)·falloff
// where falloff uses smoothstep to taper from 1 at coast to 0 at threshold.
func ApplyCoastal(g *CoastalGen, dx, dy, dz, height, distToCoast, amp, threshold, freq float64) float64 {
	if amp <= 0 || distToCoast >= threshold || threshold <= 0 {
		return height
	}

	// Smooth falloff from 1 at coast (dist=0) to 0 at threshold.
	t := 1.0 - distToCoast/threshold
	falloff := t * t * (3 - 2*t) // smoothstep

	// Sample three octaves at different frequencies.
	n4 := g.g.FractalNoise3D(dx, dy, dz, 4, 2.0, 0.5, freq*4)
	n5 := g.g.FractalNoise3D(dx, dy, dz, 4, 2.0, 0.5, freq*8)
	n6 := g.g.FractalNoise3D(dx, dy, dz, 4, 2.0, 0.5, freq*16)

	// Combine the noise samples and center around 0.
	// Each normalized fBm is in [0,1], so average contribution is (1 + 0.5 + 0.25)/2 = 0.875.
	delta := (n4 + n5/2 + n6/4) - (1 + 0.5 + 0.25)/2

	// Apply the formula: amp*(1-e⁴)*delta*falloff
	e4 := height * height * height * height
	result := height + amp*(1-e4)*delta*falloff

	return clampCoastal(result, 0, 1)
}

// clampCoastal clamps a value to [lo, hi].
func clampCoastal(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
