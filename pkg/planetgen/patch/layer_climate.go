package patch

import (
	"math"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/biome"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
)

// applyClimate (layer 9): production climate formula (biome/whittaker.go
// GenerateClimateFields — climate noise is UNJITTERED), plus the
// rain-shadow multiplier via the shared per-pixel walk against the
// window-clamped height sampler (patch-local winds, spec §7).
func applyClimate(ctx *Context, st *State) *State {
	w := ctx.Fields.Window
	cc := ctx.Profile.ControlConfig
	tGen := noise.New(seed.Domain(ctx.Master, "biome.temperature"))
	mGen := noise.New(seed.Domain(ctx.Master, "biome.humidity"))

	ns := *st
	ns.T = NewGrid(w.Size)
	ns.M = NewGrid(w.Size)
	ns.RainMult = NewGrid(w.Size)
	sampler := w.Sampler(st.Height)
	rsCfg := ctx.Profile.RainShadow
	for iy := range w.Size {
		for ix := range w.Size {
			i := iy*w.Size + ix
			dx, dy, dz := w.Dir(ix, iy)
			tn := tGen.FractalNoise3D(dx, dy, dz, cc.Temperature.Octaves, cc.Temperature.Lacunarity, cc.Temperature.Persistence, cc.Temperature.Freq) * cc.Temperature.Amp
			lat := math.Asin(dy)
			latBias := 0.5 + 0.5*math.Cos(lat)*0.6
			tv := tn*0.7 + latBias*0.3
			if tv < 0 {
				tv = 0
			} else if tv > 1 {
				tv = 1
			}
			ns.T.Data[i] = tv
			ns.M.Data[i] = mGen.FractalNoise3D(dx, dy, dz, cc.Humidity.Octaves, cc.Humidity.Lacunarity, cc.Humidity.Persistence, cc.Humidity.Freq) * cc.Humidity.Amp
			if rsCfg.WalkSteps > 0 {
				ns.RainMult.Data[i] = biome.RainShadowMultiplierAt(sampler, dx, dy, dz, rsCfg)
			} else {
				ns.RainMult.Data[i] = 1
			}
		}
	}
	return &ns
}
