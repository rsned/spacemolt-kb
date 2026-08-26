// Package kblegacy reads data/legacy.json, the list of catalog entries the game
// no longer sells.
//
// The knowledge DB accumulates and never deletes, and `last_updated_tick` is 0
// on every module, so the DB alone cannot tell a live entry from a retired one.
// scripts/build_legacy.py derives that by diffing the DB against the newest
// dated API snapshot, and dates each removal by walking the snapshots back.
//
// Legacy means *not purchasable*, NOT *not usable*: players still fly a Deeprock
// Harvester and still fit Nanofiber Internal Structure. Pages for these must
// stay visible and clearly marked, never hidden.
package kblegacy

import (
	"encoding/json"
	"fmt"
	"os"
)

// Entry is one retired id.
type Entry struct {
	Name string `json:"name"`
	// LastInCatalog is the snapshot directory it was last seen in ("20260712"),
	// or empty when it predates the snapshots or never appeared in that catalog.
	LastInCatalog string `json:"last_in_catalog"`
	// Aliases are other ids sharing this display name — the game renames ids
	// (mining_cruiser -> deeprock_harvester) and the DB keeps both sides.
	Aliases []string `json:"aliases,omitempty"`
	// Fittable marks items that occupy a module slot.
	Fittable bool `json:"fittable,omitempty"`
}

// Date renders LastInCatalog as YYYY-MM-DD for display, or "" if unknown.
func (e Entry) Date() string {
	if len(e.LastInCatalog) != 8 {
		return ""
	}
	return e.LastInCatalog[0:4] + "-" + e.LastInCatalog[4:6] + "-" + e.LastInCatalog[6:8]
}

// Set is the parsed sidecar.
type Set struct {
	Ships map[string]Entry `json:"ships"`
	Items map[string]Entry `json:"items"`
}

// Ship reports whether a ship id is retired, with its entry.
func (s Set) Ship(id string) (Entry, bool) { e, ok := s.Ships[id]; return e, ok }

// Item reports whether an item id is retired, with its entry.
func (s Set) Item(id string) (Entry, bool) { e, ok := s.Items[id]; return e, ok }

// Load reads the sidecar. A missing file is not an error — the KB still builds,
// just without legacy marking — so a fresh checkout that has not run
// scripts/build_legacy.py yet does not fail the whole generation.
func Load(path string) (Set, error) {
	var s Set
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Set{Ships: map[string]Entry{}, Items: map[string]Entry{}}, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return s, fmt.Errorf("%s: %w", path, err)
	}
	if s.Ships == nil {
		s.Ships = map[string]Entry{}
	}
	if s.Items == nil {
		s.Items = map[string]Entry{}
	}
	return s, nil
}
