package planetgen

import (
	"fmt"
	"hash/fnv"
	"image"
)

// DefaultWidth is the default output width in pixels.
const DefaultWidth = 2000

// DefaultHeight is the default output height in pixels.
const DefaultHeight = 1000

// Generate creates a planet surface map for the given planet type and name.
// The planet name is hashed to produce a deterministic seed.
func Generate(planetType, planetName string, width, height int) (*image.RGBA, error) {
	profile := GetProfile(planetType)
	if profile == nil {
		return nil, fmt.Errorf("unknown planet type: %s", planetType)
	}

	seed := hashSeed(planetName)

	switch profile.Renderer {
	case "rocky":
		return RenderRocky(profile, seed, width, height), nil
	case "gas_giant":
		return RenderGasGiant(profile, seed, width, height), nil
	default:
		return nil, fmt.Errorf("unknown renderer: %s", profile.Renderer)
	}
}

// hashSeed converts a planet name to a deterministic int64 seed.
func hashSeed(name string) int64 {
	h := fnv.New64a()
	h.Write([]byte(name))
	return int64(h.Sum64())
}

// HashSeedPublic is the exported version of hashSeed for test tooling.
func HashSeedPublic(name string) int64 {
	return hashSeed(name)
}
