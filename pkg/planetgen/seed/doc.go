// Package seed provides deterministic seed derivation from planet
// names. Same name always produces the same int64 seed via fnv-64a.
//
// Phase 1+ extends this with named-domain seed mixing
// (subSeed = master ^ Hash(domain)) so each subsystem (warp, plates,
// biome, ...) gets its own deterministic sub-seed without colliding.
package seed
