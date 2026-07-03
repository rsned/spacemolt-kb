package biome

import (
	"image/color"
	"math"

	planetcolor "github.com/rsned/spacemolt-kb/pkg/planetgen/color"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// LookupColor samples a biome table at climate point (T, M) ∈ [0,1]² and
// returns the surface color at the given heightmap value. The four
// neighboring cells are bilinearly OkLab-blended.
func LookupColor(table types.BiomeTable, T, M, height float64) color.RGBA {
	if table.TBuckets == 0 || table.MBuckets == 0 || len(table.Cells) == 0 {
		return color.RGBA{128, 128, 128, 255}
	}
	T = math.Max(0, math.Min(1, T))
	M = math.Max(0, math.Min(1, M))
	height = math.Max(0, math.Min(1, height))

	tFloat := T * float64(table.TBuckets-1)
	mFloat := M * float64(table.MBuckets-1)
	t0 := int(math.Floor(tFloat))
	m0 := int(math.Floor(mFloat))
	t1 := min(t0+1, table.TBuckets-1)
	m1 := min(m0+1, table.MBuckets-1)
	tx := tFloat - math.Floor(tFloat)
	my := mFloat - math.Floor(mFloat)

	c00 := cellColor(table.Cells[t0][m0], height)
	c10 := cellColor(table.Cells[t1][m0], height)
	c01 := cellColor(table.Cells[t0][m1], height)
	c11 := cellColor(table.Cells[t1][m1], height)

	cT0 := planetcolor.BlendOkLab(c00, c10, tx)
	cT1 := planetcolor.BlendOkLab(c01, c11, tx)
	return planetcolor.BlendOkLab(cT0, cT1, my)
}

func cellColor(cell types.BiomeCell, height float64) color.RGBA {
	low := color.RGBA{R: cell.Low.R, G: cell.Low.G, B: cell.Low.B, A: 255}
	high := color.RGBA{R: cell.High.R, G: cell.High.G, B: cell.High.B, A: 255}
	return planetcolor.BlendOkLab(low, high, height)
}
