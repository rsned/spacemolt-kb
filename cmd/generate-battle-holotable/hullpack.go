package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/rsned/spacemolt-kb/pkg/footprint"
)

// Replay is the part of the exported replay model the generator reads. It is a
// local mirror rather than an import of the spacemolt repo's pkg/battlereplay:
// the generator only needs to know which hulls appear, and the page passes the
// rest of the JSON through untouched.
type Replay struct {
	BattleID     string        `json:"battle_id"`
	SystemName   string        `json:"system_name"`
	TickCount    int           `json:"tick_count"`
	Participants []Participant `json:"participants"`
}

// Participant is one combatant's identity in the replay model.
type Participant struct {
	PlayerID  string `json:"player_id"`
	Username  string `json:"username"`
	Kind      string `json:"kind"`
	SideID    int    `json:"side_id"`
	ShipClass string `json:"ship_class"`
}

// Hull is one drawable ship class, everything the renderer needs to draw it and
// nothing else. The renderer never reads a catalog or an SVG file; it reads
// this.
type Hull struct {
	// Ship is the class id, empty for a station.
	Ship string `json:"ship"`
	// D is the SVG path geometry, empty when Kind is not "hull".
	D string `json:"d,omitempty"`
	// Height is the source viewBox height. With width fixed at 1020, the draw
	// transform needs it to find the hull centre at (510, Height/2).
	Height float64 `json:"height,omitempty"`
	Aspect float64 `json:"aspect,omitempty"`
	// Scale is the catalog hull scale, 1..5, and sets relative draw size.
	Scale int `json:"scale"`
	// Kind is how to draw it: "hull", "station", or "missing".
	Kind string `json:"kind"`
	// FrameAmbiguous forwards the pipeline's own uncertainty about the hull's
	// frame, so a debug view can surface it instead of silently trusting it.
	FrameAmbiguous bool `json:"frame_ambiguous,omitempty"`
	// KBMatch is the art-to-catalog join provenance. "fuzzy" was inferred by
	// hand and is the first thing to suspect if a hull looks wrong.
	KBMatch string `json:"kb_match,omitempty"`
}

// hull kinds.
const (
	kindHull    = "hull"
	kindStation = "station"
	kindMissing = "missing"
)

// defaultScale keeps a hull the catalog has no row for from rendering at zero
// size. A ship drawn slightly wrong is debuggable; one drawn invisibly is not.
const defaultScale = 1

// BuildHullPack resolves every distinct ship class in rep to a drawable Hull.
// Classes with no art are included with Kind "missing" rather than omitted, so
// the renderer can draw a marked chevron and the operator can see the gap.
//
// The second return value is every footprint.Check problem found, keyed by
// ship class. Parse deliberately does not validate the asset contract, so a
// caller can still render a slightly off asset — but something has to do the
// reporting, or a drifted footprint (say, a viewBox width off the renderer's
// hardcoded FOOTPRINT_WIDTH) renders silently mis-centred forever. Check is
// the thing that catches that; main.go logs whatever it finds.
func BuildHullPack(rep Replay, dir string, scales map[string]int) (map[string]Hull, map[string][]string, error) {
	pack := make(map[string]Hull)
	problems := make(map[string][]string)

	for _, p := range rep.Participants {
		if _, seen := pack[p.ShipClass]; seen {
			continue
		}

		scale := scales[p.ShipClass]
		if scale <= 0 {
			scale = defaultScale
		}

		// A station carries an empty ship_class and has no footprint art; the
		// renderer draws it as a fixed glyph.
		if p.ShipClass == "" || p.Kind == kindStation {
			pack[p.ShipClass] = Hull{Ship: p.ShipClass, Kind: kindStation, Scale: scale}
			continue
		}

		path := filepath.Join(dir, p.ShipClass+".svg")
		data, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			pack[p.ShipClass] = Hull{Ship: p.ShipClass, Kind: kindMissing, Scale: scale}
			continue
		}
		if err != nil {
			return nil, nil, fmt.Errorf("read footprint for %q: %w", p.ShipClass, err)
		}

		f, err := footprint.Parse(data)
		if err != nil {
			return nil, nil, fmt.Errorf("parse footprint for %q: %w", p.ShipClass, err)
		}

		if probs := footprint.Check(f, path); len(probs) > 0 {
			problems[p.ShipClass] = probs
		}

		pack[p.ShipClass] = Hull{
			Ship:           p.ShipClass,
			D:              f.D,
			Height:         f.Height,
			Aspect:         f.Aspect,
			Scale:          scale,
			Kind:           kindHull,
			FrameAmbiguous: f.FrameAmbiguous,
			KBMatch:        f.KBMatch,
		}
	}

	return pack, problems, nil
}
