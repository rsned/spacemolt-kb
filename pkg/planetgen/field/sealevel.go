package field

import "github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"

// QuantileSeaLevel returns the height threshold such that oceanFrac of
// the heightmap's pixels fall below it, computed from a 4096-bucket
// histogram over [0,1] with linear interpolation inside the crossing
// bucket. Heights outside [0,1] are clamped into the histogram range
// (the rocky pipeline normalizes before calling this). oceanFrac ≤ 0
// returns 0 (no ocean); oceanFrac ≥ 1 returns 1.
func QuantileSeaLevel(hm *cubemap.CubeMapF, oceanFrac float64) float64 {
	if oceanFrac <= 0 {
		return 0
	}
	if oceanFrac >= 1 {
		return 1
	}
	const buckets = 4096
	var hist [buckets]int
	total := 0
	for f := range hm.Faces {
		for _, h := range hm.Faces[f] {
			b := int(h * buckets)
			if b < 0 {
				b = 0
			}
			if b >= buckets {
				b = buckets - 1
			}
			hist[b]++
			total++
		}
	}
	target := oceanFrac * float64(total)
	cum := 0.0
	for b := range buckets {
		next := cum + float64(hist[b])
		if next >= target {
			frac := 0.0
			if hist[b] > 0 {
				frac = (target - cum) / float64(hist[b])
			}
			return (float64(b) + frac) / buckets
		}
		cum = next
	}
	return 1
}
