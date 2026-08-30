// Package wildlife loads the scanned wildlife field guide from the knowledge
// database — species, sightings, attacks, kills — and turns the raw sighting
// ledger into per-system population estimates for the KB pages.
package wildlife

import (
	"math"
	"strings"

	"github.com/rsned/spacemolt-kb/pkg/galaxymap"
)

// Species is one scanned species with everything the KB knows about it.
type Species struct {
	ID           string // e.g. "rainbow_leviathan"
	Name         string
	Role         string // grazer, predator, scavenger
	MaxHull      int
	MaxShield    int
	Danger       string
	Habitats     []string
	Ranchable    bool
	ScanTraits   string
	ScanRevealed string
	FirstSeen    string // RFC3339 UTC or ""
	LastSeen     string

	// Description is the species' official lore, returned only by scanning a
	// creature (server v0.571.0). CodexSource says where it came from: "db"
	// when the knowledge DB carries it, "codex" when it was hand-recorded in
	// data/wildlife/codex.json, "" when unknown.
	Description string
	CodexSource string
	CodexTick   int

	Places  []Place  // systems where it has been sighted, sorted by name
	Attacks []Attack // observed natural weapons, aggregated
	Kills   []Kill   // recorded kills, newest first
}

// EstimatedTotal sums the latest per-system estimates.
func (s Species) EstimatedTotal() int {
	n := 0
	for _, p := range s.Places {
		n += p.Count
	}
	return n
}

// SystemCount is the number of systems with a sighting.
func (s Species) SystemCount() int { return len(s.Places) }

// Place is a species' presence in one system: the latest system-level survey
// count (or the sum of the latest per-POI counts when no system survey
// exists) plus the per-POI breakdown.
type Place struct {
	SystemID   string
	SystemName string
	Count      int    // estimated creatures
	Abundance  string // scarce / moderate / abundant, from the system survey
	Bloom      string // bloom status when reported
	Ranched    bool
	Branded    bool
	InCombat   bool
	LastTick   int
	LastSeen   string // RFC3339 UTC
	Sightings  int    // raw sighting rows behind this estimate
	POIs       []POISighting
}

// POISighting is the latest count at one POI.
type POISighting struct {
	ID, Name string
	Type     string
	Count    int
	Bloom    string
	LastTick int
}

// Attack aggregates the observed use of one natural weapon.
type Attack struct {
	Weapon      string
	DamageType  string
	ShotKind    string
	Battles     int
	Shots       int
	Hits        int
	DamageTotal float64
	DamageMin   float64 // smallest non-zero per-hit damage seen
	DamageMax   float64
	LastSeen    string
}

// Accuracy is hits as a percentage of shots, rounded.
func (a Attack) Accuracy() float64 {
	if a.Shots == 0 {
		return 0
	}
	return math.Round(100 * float64(a.Hits) / float64(a.Shots))
}

// DamagePerHit is the mean damage of a landed hit, to two decimals.
func (a Attack) DamagePerHit() float64 {
	if a.Hits == 0 {
		return 0
	}
	return math.Round(100*a.DamageTotal/float64(a.Hits)) / 100
}

// Kill is one recorded kill with its drops.
type Kill struct {
	CreatureID    string
	SystemID      string
	SystemName    string
	POIID         string
	POIName       string
	DurationTicks int
	DamageDealt   int
	DamageTaken   int
	SalvageValue  int
	KilledAt      string
	Drops         []Drop
}

// Drop is one item recovered from a kill.
type Drop struct {
	ItemID       string
	ItemName     string
	ItemCategory string
	Quantity     float64
}

// Coverage summarises how much of the galaxy the wildlife surveys have looked at.
type Coverage struct {
	TotalSystems        int
	SystemsSurveyed     int // systems with at least one survey (even empty)
	PlacesSurveyed      int // distinct system/POI places surveyed
	SystemsWithWildlife int
}

// Guide is the whole loaded field guide.
type Guide struct {
	Species  []Species // sorted by role rank then name
	Coverage Coverage

	MapSystems []*galaxymap.System
	MapByID    map[string]*galaxymap.System
}

// EstimatedCreatures sums every species' estimate.
func (g *Guide) EstimatedCreatures() int {
	n := 0
	for i := range g.Species {
		n += g.Species[i].EstimatedTotal()
	}
	return n
}

// RoleRank orders classes for display: predators are what readers look up
// first after the stock animals.
func RoleRank(role string) int {
	switch strings.ToLower(role) {
	case "grazer":
		return 0
	case "predator":
		return 1
	case "scavenger":
		return 2
	}
	return 3
}

// Slug is the page/anchor identifier for a species.
func Slug(id string) string { return strings.ReplaceAll(strings.ToLower(id), "_", "-") }
