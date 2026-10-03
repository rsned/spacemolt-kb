package tectonics

import "math"

// hash3 mixes an integer lattice point and a seed into [0,1).
func hash3(seed uint64, ix, iy, iz int64) float64 {
	h := seed ^ (uint64(ix) * 0x9E3779B97F4A7C15) ^ (uint64(iy) * 0xC2B2AE3D27D4EB4F) ^ (uint64(iz) * 0x165667B19E3779F9)
	h ^= h >> 31
	h *= 0xBF58476D1CE4E5B9
	h ^= h >> 29

	return float64(h>>11) / float64(1<<53)
}

func smooth(t float64) float64 { return t * t * (3 - 2*t) }

// valueNoise is trilinear value noise on an integer lattice, in [0,1].
func valueNoise(seed uint64, x, y, z float64) float64 {
	fx, fy, fz := math.Floor(x), math.Floor(y), math.Floor(z)
	ix, iy, iz := int64(fx), int64(fy), int64(fz)
	tx, ty, tz := smooth(x-fx), smooth(y-fy), smooth(z-fz)
	lerp := func(a, b, t float64) float64 { return a + (b-a)*t }
	c := func(dx, dy, dz int64) float64 { return hash3(seed, ix+dx, iy+dy, iz+dz) }
	x00 := lerp(c(0, 0, 0), c(1, 0, 0), tx)
	x10 := lerp(c(0, 1, 0), c(1, 1, 0), tx)
	x01 := lerp(c(0, 0, 1), c(1, 0, 1), tx)
	x11 := lerp(c(0, 1, 1), c(1, 1, 1), tx)

	return lerp(lerp(x00, x10, ty), lerp(x01, x11, ty), tz)
}

// GenerateThickness builds the crust thickness field: warped low-frequency
// value noise, normalised to [0,1], then relaxed so no 4-neighbour pair
// differs by more than p.MaxNeighborDelta.
func GenerateThickness(p Params, master int64) *Grid[float64] {
	S := p.Face
	th := NewGrid[float64](S)
	dirs := Dirs(S)
	base := newRNG(master, "thickness.base").Uint64()
	warp := newRNG(master, "thickness.warp").Uint64()
	const off = 1000.0 // keep lattice coords positive and away from the origin
	for i, d := range dirs {
		wx := (valueNoise(warp, off+d[0]*p.NoiseFreq, off+d[1]*p.NoiseFreq, off+d[2]*p.NoiseFreq) - 0.5) * 2 * p.WarpAmp
		wy := (valueNoise(warp+1, off+d[0]*p.NoiseFreq, off+d[1]*p.NoiseFreq, off+d[2]*p.NoiseFreq) - 0.5) * 2 * p.WarpAmp
		wz := (valueNoise(warp+2, off+d[0]*p.NoiseFreq, off+d[1]*p.NoiseFreq, off+d[2]*p.NoiseFreq) - 0.5) * 2 * p.WarpAmp
		x, y, z := d[0]+wx, d[1]+wy, d[2]+wz
		sum, amp, freq, norm := 0.0, 1.0, p.NoiseFreq, 0.0
		for o := range p.NoiseOctaves {
			sum += amp * valueNoise(base+uint64(o), off+x*freq, off+y*freq, off+z*freq)
			norm += amp
			amp *= 0.5
			freq *= 2
		}
		th.Cells[i] = sum/norm + p.CrustBias
	}
	normalize(th)
	for range 200 {
		if Relax(th, p.MaxNeighborDelta, 1) == 0 {
			break
		}
	}

	return th
}

func normalize(th *Grid[float64]) {
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, v := range th.Cells {
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	if hi-lo < 1e-12 {
		return
	}
	for i, v := range th.Cells {
		th.Cells[i] = (v - lo) / (hi - lo)
	}
}

// Relax runs iters Gauss-Seidel passes pulling every 4-neighbour pair that
// differs by more than maxDelta toward each other. Returns the number of
// pairs adjusted in the last pass; 0 means the bound holds everywhere.
func Relax(th *Grid[float64], maxDelta float64, iters int) int {
	nb := Neighbors4(th.S)
	fixed := 0
	for range iters {
		fixed = 0
		for i := range th.Len() {
			for _, j := range nb[i] {
				d := th.Cells[i] - th.Cells[j]
				if d > maxDelta {
					e := (d - maxDelta) / 2
					th.Cells[i] -= e
					th.Cells[j] += e
					fixed++
				} else if d < -maxDelta {
					e := (-d - maxDelta) / 2
					th.Cells[i] += e
					th.Cells[j] -= e
					fixed++
				}
			}
		}
	}

	return fixed
}
