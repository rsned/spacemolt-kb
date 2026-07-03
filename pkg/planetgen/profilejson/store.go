package profilejson

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// LoadForPlanet reads the envelope file at rootDir/<PlanetSlug(name)>.json.
// Returns (env, true, nil) on success, (nil, false, nil) when the file
// does not exist, and (nil, false, err) on any other failure (decode
// error, permission denied, etc.). An empty rootDir always returns
// (nil, false, nil) — callers use this to disable disk lookup (e.g. the
// wasm build).
func LoadForPlanet(rootDir, name string) (*Envelope, bool, error) {
	if rootDir == "" {
		return nil, false, nil
	}
	slug := PlanetSlug(name)
	if slug == "" {
		return nil, false, fmt.Errorf("profilejson: empty slug for name %q", name)
	}
	path := filepath.Join(rootDir, slug+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("profilejson: read %s: %w", path, err)
	}
	env, err := Decode(data)
	if err != nil {
		return nil, false, fmt.Errorf("profilejson: decode %s: %w", path, err)
	}
	return env, true, nil
}
