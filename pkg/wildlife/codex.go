package wildlife

import (
	"encoding/json"
	"errors"
	"os"
)

// CodexEntry is a species' official lore as returned by scanning a creature
// (server v0.571.0 adds a `description` field to creature scans). The server
// does not store it for the scanner, so entries read by hand are kept in a
// codex file until the knowledge DB carries the column.
type CodexEntry struct {
	Description string `json:"description"`
	ScannedTick int    `json:"scanned_tick,omitempty"`
	ScannedUTC  string `json:"scanned_utc,omitempty"`
	Source      string `json:"source,omitempty"`
}

// Codex maps species id to its hand-recorded entry.
type Codex map[string]CodexEntry

// LoadCodex reads a codex file. Keys starting with "_" are notes and are
// skipped; a missing file is an empty codex.
func LoadCodex(path string) (Codex, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Codex{}, nil
	}
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	out := Codex{}
	for k, v := range raw {
		if len(k) == 0 || k[0] == '_' {
			continue
		}
		var e CodexEntry
		if err := json.Unmarshal(v, &e); err != nil {
			return nil, err
		}
		if e.Description != "" {
			out[k] = e
		}
	}
	return out, nil
}

// Apply fills in Description from the codex for species that have none from
// the database, marking them CodexSource "codex". A DB description wins.
func (c Codex) Apply(species []Species) {
	for i := range species {
		if species[i].Description != "" {
			species[i].CodexSource = "db"
			continue
		}
		if e, ok := c[species[i].ID]; ok {
			species[i].Description = e.Description
			species[i].CodexSource = "codex"
			species[i].CodexTick = e.ScannedTick
		}
	}
}
