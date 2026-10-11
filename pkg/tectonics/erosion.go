package tectonics

import "math"

// erode is step 8 of Step: land pixels (elevation at or above seaLevel)
// give thickness to each lower 4-neighbour in proportion to the elevation
// drop, capped at ErosionMaxPerStep of their outflow fractions per step.
// Moves are computed from the step's starting thickness and applied in one
// pass, so the result is order-independent. Material is conserved except
// that a deep-ocean receiver keeps only SedimentOceanFactor of a slice.
func (s *State) erode(th *Grid[float64], age *Grid[uint16], seaLevel float64) {
	p := s.P
	if p.ErosionRate <= 0 {
		return
	}
	elev := Elevation(th, age, p)
	nb := Neighbors4(th.S)
	delta := make([]float64, th.Len())
	k := p.ErosionRate * p.MyrPerStep() / p.ElevPeakKm
	for i, hi := range elev.Cells {
		if hi < seaLevel {
			continue
		}
		var f [4]float64
		total := 0.0
		for n, j := range nb[i] {
			if hj := elev.Cells[j]; hj < hi {
				f[n] = k * (hi - hj)
				total += f[n]
			}
		}
		if total <= 0 {
			continue
		}
		scale := 1.0
		if total > p.ErosionMaxPerStep {
			scale = p.ErosionMaxPerStep / total
		}
		for n, j := range nb[i] {
			if f[n] == 0 {
				continue
			}
			d := f[n] * scale * math.Max(0, th.Cells[i]-th.Cells[j])
			if d == 0 {
				continue
			}
			delta[i] -= d
			if th.Cells[j] < p.ContinentalThreshold && elev.Cells[j] < seaLevel {
				d *= p.SedimentOceanFactor
			}
			delta[j] += d
		}
	}
	for i := range th.Cells {
		th.Cells[i] = math.Max(0, math.Min(1, th.Cells[i]+delta[i]))
	}
}
