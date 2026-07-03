package color

import (
	_ "embed"
	"fmt"
)

//go:embed luts/scorched.cube
var lutScorchedRaw string

//go:embed luts/arid.cube
var lutAridRaw string

//go:embed luts/terran.cube
var lutTerranRaw string

//go:embed luts/tundra.cube
var lutTundraRaw string

//go:embed luts/glacial.cube
var lutGlacialRaw string

//go:embed luts/ice_world.cube
var lutIceWorldRaw string

//go:embed luts/super_terran.cube
var lutSuperTerranRaw string

//go:embed luts/hothouse.cube
var lutHothouseRaw string

//go:embed luts/lava_world.cube
var lutLavaWorldRaw string

//go:embed luts/oceanic.cube
var lutOceanicRaw string

//go:embed luts/jovian.cube
var lutJovianRaw string

//go:embed luts/ice_giant.cube
var lutIceGiantRaw string

//go:embed luts/unknown.cube
var lutUnknownRaw string

// lutRegistry holds the parsed archetype LUTs keyed by archetype name.
var lutRegistry = map[string]*LUT{}

func init() {
	register := func(name, raw string) {
		lut, err := ParseCubeLUT(raw)
		if err != nil {
			panic(fmt.Sprintf("color: parse LUT %q: %v", name, err))
		}
		lut.Name = name
		lutRegistry[name] = &lut
	}
	register("scorched", lutScorchedRaw)
	register("arid", lutAridRaw)
	register("terran", lutTerranRaw)
	register("tundra", lutTundraRaw)
	register("glacial", lutGlacialRaw)
	register("ice_world", lutIceWorldRaw)
	register("super_terran", lutSuperTerranRaw)
	register("hothouse", lutHothouseRaw)
	register("lava_world", lutLavaWorldRaw)
	register("oceanic", lutOceanicRaw)
	register("jovian", lutJovianRaw)
	register("ice_giant", lutIceGiantRaw)
	register("unknown", lutUnknownRaw)
}

// LookupLUT returns the named archetype LUT, or nil if no such LUT.
// The returned pointer aliases the package-internal copy — do not mutate.
func LookupLUT(name string) *LUT {
	return lutRegistry[name]
}
