// Package feature: civilization stage. This file owns the population-
// assignment, render-orchestration, and civ-specific glue logic for
// the Phase 9b civilization-overlay pipeline. The concrete renderable
// artifacts (habitability scalar field, Bridson sites, road network,
// city/agriculture/night-light passes) live in sibling files within
// this package; civ.go is the top-level coordinator.
package feature

import (
	"math"
	"sort"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// zipfAlpha is the Zipf exponent applied to ranked civilization sites.
// 1.07 is the typical empirical value for real-world city-size
// distributions (Gabaix 1999, "Zipf's Law for Cities").
const zipfAlpha = 1.07

// AssignPopulations sorts sites by habitability descending and writes
// Zipfian populations to each site (mutating the slice in place).
// population[rank] = cfg.Tier * (rank+1)^(-zipfAlpha), capped at
// cfg.MaxPopulation when MaxPopulation > 0.
//
// When sites is empty or cfg.Tier <= 0, AssignPopulations is a no-op
// (matching the "Tier == 0 disables civ" idiom).
func AssignPopulations(sites []Site, cfg types.CivConfig) {
	if len(sites) == 0 || cfg.Tier <= 0 {
		return
	}

	// Stable sort so habitability ties resolve by original-slice
	// order — required for deterministic outputs given identical
	// inputs.
	sort.SliceStable(sites, func(i, j int) bool {
		return sites[i].Habitability > sites[j].Habitability
	})

	for i := range sites {
		pop := cfg.Tier * math.Pow(float64(i+1), -zipfAlpha)
		if cfg.MaxPopulation > 0 && pop > cfg.MaxPopulation {
			pop = cfg.MaxPopulation
		}
		sites[i].Population = pop
	}
}
