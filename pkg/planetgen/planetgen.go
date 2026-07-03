package planetgen

import (
	"fmt"
	"image"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/profilejson"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/render"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// DefaultFaceSize is the default cube-map face edge length in pixels.
const DefaultFaceSize = 1024

// DefaultWidth is the default equirect output width in pixels.
const DefaultWidth = 2000

// DefaultHeight is the default equirect output height in pixels.
const DefaultHeight = 1000

// profileRoot is the directory checked for per-planet envelope files.
// Empty string disables disk lookup (used by the wasm build).
var profileRoot = "data/planet-profiles"

// SetProfileRoot overrides the directory consulted by Generate before
// it falls back to GetProfile. Tests pass t.TempDir(); the wasm
// entrypoint passes "".
func SetProfileRoot(dir string) { profileRoot = dir }

// ProfileRootForTest returns the current profileRoot. Intended only for
// tests that need to save and restore the value.
func ProfileRootForTest() string { return profileRoot }

// Generate creates a planet cube map for the given planet type and name.
// The planet name is hashed to produce a deterministic seed. If a
// per-planet envelope file exists under the profile root, its inner
// profile is used instead of the in-code default for that type.
func Generate(planetType, planetName string, faceSize int) (*cubemap.CubeMap, error) {
	profile, err := resolveProfile(planetType, planetName)
	if err != nil {
		return nil, err
	}
	s := hashSeed(planetName)
	switch profile.Renderer {
	case "rocky":
		return render.RenderRocky(profile, s, faceSize), nil
	case "gas_giant":
		return render.RenderGasGiant(profile, s, faceSize), nil
	default:
		return nil, fmt.Errorf("unknown renderer: %s", profile.Renderer)
	}
}

// resolveProfile picks the active PlanetProfile for a (type, name)
// pair: first the on-disk envelope under profileRoot, otherwise the
// in-code GetProfile default. Type-mismatch between envelope and the
// requested type is a hard error.
func resolveProfile(planetType, planetName string) (*types.PlanetProfile, error) {
	if profileRoot != "" {
		env, ok, err := profilejson.LoadForPlanet(profileRoot, planetName)
		if err != nil {
			return nil, err
		}
		if ok {
			if env.Type != planetType {
				return nil, fmt.Errorf(
					"profile type mismatch for %q: file=%q, requested=%q",
					planetName, env.Type, planetType)
			}
			return env.Profile, nil
		}
	}
	p := GetProfile(planetType)
	if p == nil {
		return nil, fmt.Errorf("unknown planet type: %s", planetType)
	}
	return p, nil
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
