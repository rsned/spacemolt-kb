package tectonics

import "math"

const (
	seaMinKm = -32.0
	seaBins  = 65536 // 1 m bins over [-32 km, +33.536 km)
	seaBinKm = 0.001
)

// drawWaterFactor is the planet's seeded multiplier on its step-0 hollow volume.
func drawWaterFactor(p Params, master int64) float64 {
	rng := newRNG(master, "water.factor")
	return p.WaterFactorMin + rng.Float64()*(p.WaterFactorMax-p.WaterFactorMin)
}

// HollowVolume is the volume in km³ between the height datum (km) and every
// pixel below it, weighting pixels by their true area on the sphere.
func HollowVolume(elev *Grid[float64], datum float64, p Params) float64 {
	w := AreaWeights(elev.S)
	r2 := p.RadiusKm * p.RadiusKm
	v := 0.0
	for i, h := range elev.Cells {
		if h < datum {
			v += (datum - h) * w[i] * r2
		}
	}
	return v
}

func seaBin(h float64) int {
	b := int(math.Floor((h - seaMinKm) / seaBinKm))
	return max(0, min(seaBins-1, b))
}

// SeaLevelFor returns the height L in km at which the hollows of elev hold
// water km³. Heights are quantised to 1 m bins; the volume is linear in L
// inside a bin, so L is interpolated there. Zero water gives the lowest
// pixel; more water than the range can hold gives the top of the range.
func SeaLevelFor(elev *Grid[float64], water float64, p Params) float64 {
	w := AreaWeights(elev.S)
	r2 := p.RadiusKm * p.RadiusKm
	areaAt := make([]float64, seaBins)
	for i, h := range elev.Cells {
		areaAt[seaBin(h)] += w[i] * r2
	}
	area, volume := 0.0, 0.0
	for b := range seaBins {
		area += areaAt[b]
		if area > 0 && volume+area*seaBinKm >= water {
			return seaMinKm + float64(b)*seaBinKm + (water-volume)/area
		}
		volume += area * seaBinKm
	}
	return seaMinKm + seaBins*seaBinKm
}

// LandShare is the area-weighted share of the sphere at or above seaLevel.
func LandShare(elev *Grid[float64], seaLevel float64) float64 {
	w := AreaWeights(elev.S)
	land := 0.0
	for i, h := range elev.Cells {
		if h >= seaLevel {
			land += w[i]
		}
	}
	return land / (4 * math.Pi)
}
