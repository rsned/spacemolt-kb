package patch

import (
	"sort"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

// Candidate is a scored patch window.
type Candidate struct {
	Window Window  `json:"window"`
	Score  float64 `json:"score"`
}

// fxClassBoundaryKm: a footprint pixel closer than this to a class
// boundary counts as "active" for that class.
const fxClassBoundaryKm = 300.0

// Pick ranks candidate windows of size×size (at virtual resolution
// sProd) by tectonic interest and returns the topN. Scoring runs on
// the S_tect rasters: FX-class presence (+1 per distinct class with
// >2% active pixels), boundary density (up to +0.3 per class), and a
// +1 bonus for windows that straddle land and ocean.
func Pick(sd *SphereData, size, sProd, topN int) []Candidate {
	sT := sd.STect
	// Footprint of the window in S_tect pixels.
	fp := size * sT / sProd
	if fp < 2 {
		fp = 2
	}
	stride := max(fp/2, 1)

	classes := []*cubemap.CubeMapF{sd.FX.BeltDist, sd.FX.SubdDist, sd.FX.ArcDist, sd.FX.RidgeDist, sd.FX.RiftDist}

	var cands []Candidate
	maxOrigin := sT - fp
	for face := range cubemap.Face(cubemap.NumFaces) {
		for oy := 0; oy <= maxOrigin; oy += stride {
			for ox := 0; ox <= maxOrigin; ox += stride {
				var score float64
				total := fp * fp
				for _, cls := range classes {
					active := 0
					for fy := oy; fy < oy+fp; fy++ {
						row := cls.Faces[face][fy*sT : fy*sT+sT]
						for fx := ox; fx < ox+fp; fx++ {
							if row[fx] < fxClassBoundaryKm {
								active++
							}
						}
					}
					frac := float64(active) / float64(total)
					if frac > 0.02 {
						score += 1.0
					}
					score += min(frac, 0.3)
				}
				land := 0
				for fy := oy; fy < oy+fp; fy++ {
					row := sd.Crust.ContinentalMask.Faces[face][fy*sT : fy*sT+sT]
					for fx := ox; fx < ox+fp; fx++ {
						if row[fx] > 0.5 {
							land++
						}
					}
				}
				lf := float64(land) / float64(total)
				if lf > 0.25 && lf < 0.75 {
					score += 1.0
				}
				// Map the S_tect origin back to virtual pixels.
				w := Window{Face: face, X0: ox * sProd / sT, Y0: oy * sProd / sT, Size: size, SProd: sProd}
				if w.X0+size > sProd {
					w.X0 = sProd - size
				}
				if w.Y0+size > sProd {
					w.Y0 = sProd - size
				}
				cands = append(cands, Candidate{Window: w, Score: score})
			}
		}
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].Score != cands[j].Score {
			return cands[i].Score > cands[j].Score
		}
		a, b := cands[i].Window, cands[j].Window
		if a.Face != b.Face {
			return a.Face < b.Face
		}
		if a.Y0 != b.Y0 {
			return a.Y0 < b.Y0
		}
		return a.X0 < b.X0
	})
	if topN > 0 && len(cands) > topN {
		cands = cands[:topN]
	}
	return cands
}
