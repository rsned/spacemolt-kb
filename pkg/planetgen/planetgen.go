package planetgen

import (
	"fmt"
	"image"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/render"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
)

// DefaultFaceSize is the default cube-map face edge length in pixels.
const DefaultFaceSize = 1024

// DefaultWidth is the default equirect output width in pixels.
const DefaultWidth = 2000

// DefaultHeight is the default equirect output height in pixels.
const DefaultHeight = 1000

// Generate creates a planet cube map for the given planet type and name.
// The planet name is hashed to produce a deterministic seed.
func Generate(planetType, planetName string, faceSize int) (*cubemap.CubeMap, error) {
	profile := GetProfile(planetType)
	if profile == nil {
		return nil, fmt.Errorf("unknown planet type: %s", planetType)
	}
	seed := hashSeed(planetName)
	switch profile.Renderer {
	case "rocky":
		return render.RenderRocky(profile, seed, faceSize), nil
	case "gas_giant":
		return render.RenderGasGiant(profile, seed, faceSize), nil
	default:
		return nil, fmt.Errorf("unknown renderer: %s", profile.Renderer)
	}
}

// GenerateEquirect generates a planet and bakes it to a width×height
// equirectangular RGBA image. Convenience wrapper around Generate +
// cubemap.BakeEquirect.
func GenerateEquirect(planetType, planetName string, width, height int) (*image.RGBA, error) {
	cm, err := Generate(planetType, planetName, DefaultFaceSize)
	if err != nil {
		return nil, err
	}
	return cubemap.BakeEquirect(cm, width, height), nil
}

// hashSeed converts a planet name to a deterministic int64 seed.
// Thin wrapper around seed.Hash retained for in-package callers.
func hashSeed(name string) int64 {
	return seed.Hash(name)
}

// HashSeedPublic is the exported wrapper for external callers
// (e.g. pkg/kbdb). Phase 1+ callers should prefer pkg/planetgen/seed.Hash
// directly.
func HashSeedPublic(name string) int64 {
	return seed.Hash(name)
}
