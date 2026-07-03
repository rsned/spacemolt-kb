package seed

// Domain mixes a master seed with a named-domain string to produce a
// per-subsystem seed. Same (master, name) always returns the same value;
// adding a new subsystem (i.e., a new name) does not shift any existing
// subsystem's seed.
//
// Use named domains like "warp.x", "control.continentalness",
// "biome.temperature" — see the master plan's seed-discipline section.
func Domain(master int64, name string) int64 {
	return master ^ Hash(name)
}
