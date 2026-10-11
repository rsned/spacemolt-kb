package tectonics

import "math"

// thExpected is the thickness the oceanic thickening law in Step predicts
// after ageSteps steps of cooling from ridge crust.
func thExpected(p Params, ageSteps float64) float64 {
	return p.RidgeThickness + p.OceanicThickening*math.Sqrt(ageSteps)
}

func smoothstep(x float64) float64 {
	x = math.Max(0, math.Min(1, x))
	return x * x * (3 - 2*x)
}

// elevationAt returns the height in km of one pixel from its thickness and
// crust age: a continental curve above ContinentalThreshold, age-driven
// subsidence plus thickness relief below it, blended across ElevBlend.
func elevationAt(p Params, th float64, ageSteps uint16) float64 {
	T := p.ContinentalThreshold
	x := math.Max(0, (th-T)/(1-T))
	hc := p.ElevShelfKm + (p.ElevPeakKm-p.ElevShelfKm)*math.Pow(x, p.ElevPow)

	ageMyr := float64(ageSteps) * p.MyrPerStep()
	base := math.Max(p.AbyssKm, p.RidgeDepthKm-p.SubsidenceKm*math.Sqrt(ageMyr))
	dev := math.Max(-0.1, math.Min(0.1, th-thExpected(p, float64(ageSteps))))
	ho := base + p.OceanReliefKm*dev

	w := smoothstep((th - (T - p.ElevBlend)) / (2 * p.ElevBlend))
	return (1-w)*ho + w*hc
}

// Elevation derives the height in km of every pixel. It is a pure function
// of thickness and crust age and is never stored on State.
func Elevation(th *Grid[float64], age *Grid[uint16], p Params) *Grid[float64] {
	h := NewGrid[float64](th.S)
	for i, v := range th.Cells {
		h.Cells[i] = elevationAt(p, v, age.Cells[i])
	}
	return h
}

func clampU16(x float64) uint16 {
	if x < 0 {
		return 0
	}
	if x > math.MaxUint16 {
		return math.MaxUint16
	}
	return uint16(x)
}

// InitialAges gives step-0 crust ages: continents are ContinentAgeMyr old;
// ocean age is inferred by inverting the thickening law (thicker ocean is
// older), capped at OceanAgeCapMyr.
func InitialAges(th *Grid[float64], p Params) *Grid[uint16] {
	age := NewGrid[uint16](th.S)
	myr := p.MyrPerStep()
	cont := clampU16(math.Round(p.ContinentAgeMyr / myr))
	capSteps := p.OceanAgeCapMyr / myr
	for i, v := range th.Cells {
		if v >= p.ContinentalThreshold {
			age.Cells[i] = cont
			continue
		}
		steps := 0.0
		if v > p.RidgeThickness && p.OceanicThickening > 0 {
			d := (v - p.RidgeThickness) / p.OceanicThickening
			steps = d * d
		}
		age.Cells[i] = clampU16(math.Round(math.Min(steps, capSteps)))
	}
	return age
}
