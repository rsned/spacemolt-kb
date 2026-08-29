// Package resourcediff snapshots the values shown on the KB Resources page and
// reports how the surveyed picture changed between snapshots: new resource
// types in the catalog, types discovered for the first time, new deposits,
// new (and hidden) POIs, and re-surveyed deposits.
//
// A snapshot captures exactly what the Resources page renders, so it can be
// built either from the knowledge DB (the page's source) or by parsing a
// committed copy of the page itself (used to bootstrap history from git).
package resourcediff

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
)

// Snapshot sources.
const (
	SourceDB   = "db"   // read from the knowledge database
	SourceHTML = "html" // parsed from a generated kb/resources/index.html
)

// Summary holds the headline numbers from the page's summary cards.
type Summary struct {
	Types    int `json:"types"`    // resource types listed (discovered or not)
	Deposits int `json:"deposits"` // total deposit rows
	Systems  int `json:"systems"`  // star systems in the galaxy
	Explored int `json:"explored"` // systems with survey data
}

// ResourceType is one ore/material the page lists, discovered or not.
type ResourceType struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category,omitempty"` // "ore", "material", or "" when the item is not in the catalog
}

// Deposit is one row of the page: a resource occurrence at a POI. Numeric
// values are stored the way the page prints them (richness rounded, remaining
// truncated) so DB- and HTML-sourced snapshots compare equal.
type Deposit struct {
	SystemID   string `json:"system"`
	SystemName string `json:"system_name"`
	POIID      string `json:"poi"`
	POIName    string `json:"poi_name"`
	Hidden     bool   `json:"hidden,omitempty"`
	Station    bool   `json:"station,omitempty"` // a station exists in the system
	ResourceID string `json:"resource"`
	Richness   int    `json:"richness"`
	Remaining  int    `json:"remaining"`
	// MaxRemaining is the deposit's capacity; only get_poi reports it, so 0
	// means unknown for most rows.
	MaxRemaining int `json:"max_remaining,omitempty"`
	LastTick     int `json:"tick,omitempty"` // 0 when unknown (the page prints "-" for ticks below 400000)
}

// Key identifies a deposit across snapshots.
func (d Deposit) Key() string { return d.POIID + "|" + d.ResourceID }

// SupportedPower is the mining power the deposit accepts right now. The
// server gates mining on a ship's summed mining-module power staying below
// floor(remaining/20), so it falls as the deposit is worked.
func (d Deposit) SupportedPower() int { return SupportedPower(d.Remaining) }

// MaxSupportedPower is the ceiling at full capacity, or 0 when the capacity
// is unknown.
func (d Deposit) MaxSupportedPower() int { return SupportedPower(d.MaxRemaining) }

// SupportedPower converts a remaining amount to the mining power it supports.
func SupportedPower(remaining int) int {
	if remaining <= 0 {
		return 0
	}
	return remaining / 20
}

// Snapshot is the full captured state of the Resources page.
type Snapshot struct {
	Date          string         `json:"date"` // YYYY-MM-DD
	Source        string         `json:"source"`
	ServerVersion string         `json:"server_version,omitempty"`
	Summary       Summary        `json:"summary"`
	Types         []ResourceType `json:"types"`
	Deposits      []Deposit      `json:"deposits"`
}

// normalize sorts types and deposits into their canonical order.
func (s *Snapshot) normalize() {
	slices.SortFunc(s.Types, func(a, b ResourceType) int { return cmp.Compare(a.ID, b.ID) })
	slices.SortFunc(s.Deposits, func(a, b Deposit) int {
		return cmp.Or(
			cmp.Compare(a.ResourceID, b.ResourceID),
			cmp.Compare(a.SystemID, b.SystemID),
			cmp.Compare(a.POIID, b.POIID),
		)
	})
}

// TypeName returns the display name for a resource ID, falling back to the ID.
func (s *Snapshot) TypeName(id string) string {
	for _, t := range s.Types {
		if t.ID == id {
			return t.Name
		}
	}
	return id
}

// Marshal encodes the snapshot as JSON with one deposit per line, so that
// committed snapshots diff cleanly in git.
func (s *Snapshot) Marshal() ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("{\n")
	writeField := func(name string, v any) error {
		enc, err := json.Marshal(v)
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "  %q: %s,\n", name, enc)
		return nil
	}
	for _, f := range []struct {
		name string
		v    any
	}{
		{"date", s.Date}, {"source", s.Source}, {"server_version", s.ServerVersion}, {"summary", s.Summary},
	} {
		if err := writeField(f.name, f.v); err != nil {
			return nil, err
		}
	}
	writeList := func(name string, n int, item func(i int) any, last bool) error {
		fmt.Fprintf(&b, "  %q: [", name)
		for i := range n {
			enc, err := json.Marshal(item(i))
			if err != nil {
				return err
			}
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString("\n    ")
			b.Write(enc)
		}
		if n > 0 {
			b.WriteString("\n  ")
		}
		b.WriteString("]")
		if !last {
			b.WriteString(",")
		}
		b.WriteString("\n")
		return nil
	}
	if err := writeList("types", len(s.Types), func(i int) any { return s.Types[i] }, false); err != nil {
		return nil, err
	}
	if err := writeList("deposits", len(s.Deposits), func(i int) any { return s.Deposits[i] }, true); err != nil {
		return nil, err
	}
	b.WriteString("}\n")
	return b.Bytes(), nil
}

// Unmarshal decodes a snapshot written by Marshal.
func Unmarshal(data []byte) (*Snapshot, error) {
	var s Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	s.normalize()
	return &s, nil
}
