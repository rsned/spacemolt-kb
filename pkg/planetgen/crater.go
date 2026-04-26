package planetgen

import "math"

// applyCratersLegacy is the pre-refactor heightmap-array variant of
// crater stamping, retained until rocky.go is ported to cube-map in
// Task 12.
func applyCratersLegacy(heightmap [][]float64, craters []Crater, width, height int, depth float64) {
	for _, c := range craters {
		cx := math.Cos(c.Lat) * math.Cos(c.Lon)
		cy := math.Sin(c.Lat)
		cz := math.Cos(c.Lat) * math.Sin(c.Lon)
		latCenter := math.Pi/2 - c.Lat
		yCenter := latCenter / math.Pi * float64(height)
		xCenter := c.Lon / (2 * math.Pi) * float64(width)
		pixRadius := c.Radius / math.Pi * float64(height) * 1.5
		yMin := int(math.Max(0, yCenter-pixRadius))
		yMax := int(math.Min(float64(height-1), yCenter+pixRadius))
		for py := yMin; py <= yMax; py++ {
			xMin := int(xCenter - pixRadius)
			xMax := int(xCenter + pixRadius)
			for px := xMin; px <= xMax; px++ {
				wpx := ((px % width) + width) % width
				lon := float64(wpx) / float64(width) * 2 * math.Pi
				lat := math.Pi/2 - float64(py)/float64(height)*math.Pi
				sx := math.Cos(lat) * math.Cos(lon)
				sy := math.Sin(lat)
				sz := math.Cos(lat) * math.Sin(lon)
				dot := sx*cx + sy*cy + sz*cz
				if dot > 1 {
					dot = 1
				} else if dot < -1 {
					dot = -1
				}
				dist := math.Acos(dot)
				if dist < c.Radius {
					t := dist / c.Radius
					var mod float64
					if t < 0.8 {
						mod = -depth * (1 - (t/0.8)*(t/0.8))
					} else {
						rimT := (t - 0.8) / 0.2
						mod = depth * 0.15 * math.Sin(rimT*math.Pi)
					}
					heightmap[py][wpx] += mod
				}
			}
		}
	}
	for py := range height {
		for px := range width {
			heightmap[py][px] = math.Max(0, math.Min(1, heightmap[py][px]))
		}
	}
}
