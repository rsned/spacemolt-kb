package main

import (
	"strconv"
	"strings"
)

// WormholeEnd names the system at the other end of a wormhole.
type WormholeEnd struct {
	ID   string
	Name string
	// SeenAgo is how long ago an agent last saw this end ("3h"), measured from
	// the newest POI record. Wormholes are temporary (v0.694.0), so a link
	// that has not been seen for a while may already have collapsed.
	SeenAgo string
}

// humanAge formats a tick count as a short age: "just now", "12m", "5h", "3d".
func humanAge(ticks int) string {
	secs := ticks * secondsPerTick
	switch {
	case secs < 60:
		return "just now"
	case secs < 3600:
		return strconv.Itoa(secs/60) + "m"
	case secs < 48*3600:
		return strconv.Itoa(secs/3600) + "h"
	default:
		return strconv.Itoa(secs/86400) + "d"
	}
}

// wormholeKey returns the id the two ends of one wormhole share, and whether
// the POI is an active entrance (true) or exit (false). Two id schemes are in
// use: hashed pairs (wh_entrance_X / wh_exit_X) and named ones (X / X_exit).
// Collapsed wormholes and other POIs yield ok=false.
func wormholeKey(p SystemPOI) (key string, entrance, ok bool) {
	switch p.Type {
	case "wormhole_entrance":
		if k, found := strings.CutPrefix(p.ID, "wh_entrance_"); found {
			return k, true, true
		}
		return p.ID, true, true
	case "wormhole_exit":
		if k, found := strings.CutPrefix(p.ID, "wh_exit_"); found {
			return k, false, true
		}
		return strings.TrimSuffix(p.ID, "_exit"), false, true
	}
	return "", false, false
}

// linkWormholes points every active wormhole entrance at the system holding
// its exit, and every exit back at its entrance. Wormholes are one-way in the
// game, so these are POIs rather than jump connections; an end whose partner
// has not been recorded stays unlinked.
func linkWormholes(systems []*System) {
	type end struct {
		sys *System
		i   int
	}
	entrances, exits := map[string]end{}, map[string]end{}
	for _, s := range systems {
		for i, p := range s.POIs {
			if k, isEntrance, ok := wormholeKey(p); ok {
				if isEntrance {
					entrances[k] = end{s, i}
				} else {
					exits[k] = end{s, i}
				}
			}
		}
	}
	now := 0
	for _, s := range systems {
		for _, p := range s.POIs {
			now = max(now, p.LastUpdatedTick)
		}
	}
	age := func(p SystemPOI) string { return humanAge(max(0, now-p.LastUpdatedTick)) }
	for k, in := range entrances {
		out, ok := exits[k]
		if !ok {
			continue
		}
		in.sys.POIs[in.i].WormholeTo = &WormholeEnd{ID: out.sys.ID, Name: out.sys.Name, SeenAgo: age(in.sys.POIs[in.i])}
		out.sys.POIs[out.i].WormholeFrom = &WormholeEnd{ID: in.sys.ID, Name: in.sys.Name, SeenAgo: age(out.sys.POIs[out.i])}
	}
}
