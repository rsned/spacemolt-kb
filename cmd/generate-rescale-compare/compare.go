package main

import (
	"cmp"
	"database/sql"
	"math"
	"slices"
)

// RescaleTick is the first tick after the v0.698.0 rescale went live
// (2026-11-12 03:20 UTC, 7:20 pm PST). A system counts as rescanned once
// every body it has was recorded at or after it.
const RescaleTick = 2381475

// BodyRow is one body's distance from its star, before and after.
type BodyRow struct {
	Name, Type    string
	Before, After float64
	HasB, HasA    bool // present on that side
}

// Ratio is After/Before, or 0 when it is not meaningful.
func (r BodyRow) Ratio() float64 {
	if !r.HasB || !r.HasA || r.Before < 0.05 || r.Type == "sun" {
		return 0
	}
	return r.After / r.Before
}

// System is one system's comparison.
type System struct {
	ID, Name, Class string
	Rescanned       bool
	Before, After   []Body
	BeforeExtent    float64 // outermost body, AU from the star
	AfterExtent     float64
	Rows            []BodyRow
}

func extent(bs []Body) float64 {
	e := 0.0
	for _, b := range bs {
		e = math.Max(e, math.Hypot(b.X, b.Y))
	}
	return e
}

func starClass(bs []Body) string {
	for _, b := range bs {
		if b.Type == "sun" && b.Class != "" {
			return b.Class
		}
	}
	return ""
}

// afterLayout is a system's current bodies plus whether all were recorded
// after the rescale.
type afterLayout struct {
	Layout
	rescanned bool
}

// loadAfter reads every system's current bodies from the knowledge DB.
func loadAfter(db *sql.DB) (map[string]afterLayout, error) {
	names := map[string]string{}
	rows, err := db.Query(`SELECT id, name FROM systems`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, n string
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		names[id] = n
	}
	_ = rows.Close()

	out := map[string]afterLayout{}
	rows, err = db.Query(`SELECT system_id, name, type, COALESCE(class,''), position_x, position_y, COALESCE(last_updated_tick,0)
		FROM pois ORDER BY system_id, name`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var sid string
		var b Body
		var tick int
		if err := rows.Scan(&sid, &b.Name, &b.Type, &b.Class, &b.X, &b.Y, &tick); err != nil {
			return nil, err
		}
		a, ok := out[sid]
		if !ok {
			a = afterLayout{Layout: Layout{Name: names[sid]}, rescanned: true}
		}
		a.Bodies = append(a.Bodies, b)
		a.rescanned = a.rescanned && tick >= RescaleTick
		out[sid] = a
	}
	return out, rows.Err()
}

// compareSystems pairs every baseline system with its current layout, matching
// bodies by name. Systems are sorted by name.
func compareSystems(before map[string]Layout, after map[string]afterLayout) []System {
	var out []System
	for id, b := range before {
		a := after[id]
		s := System{ID: id, Name: b.Name, Before: b.Bodies, Rescanned: a.rescanned && len(a.Bodies) > 0}
		if s.Name == "" {
			s.Name = a.Name
		}
		s.Class = starClass(b.Bodies)
		s.BeforeExtent = extent(b.Bodies)
		if s.Rescanned {
			s.After = a.Bodies
			s.AfterExtent = extent(a.Bodies)
			if c := starClass(a.Bodies); c != "" {
				s.Class = c
			}
			s.Rows = bodyRows(b.Bodies, a.Bodies)
		}
		out = append(out, s)
	}
	slices.SortFunc(out, func(x, y System) int { return cmp.Or(cmp.Compare(x.Name, y.Name), cmp.Compare(x.ID, y.ID)) })
	return out
}

// bodyRows lists every body by distance from the star after the rescale (or
// before, for one that is gone), suns first.
func bodyRows(before, after []Body) []BodyRow {
	byName := map[string]*BodyRow{}
	var order []string
	add := func(b Body, isAfter bool) {
		r, ok := byName[b.Name]
		if !ok {
			r = &BodyRow{Name: b.Name, Type: b.Type}
			byName[b.Name] = r
			order = append(order, b.Name)
		}
		d := math.Hypot(b.X, b.Y)
		if isAfter {
			r.After, r.HasA, r.Type = d, true, b.Type
		} else {
			r.Before, r.HasB = d, true
		}
	}
	for _, b := range before {
		add(b, false)
	}
	for _, b := range after {
		add(b, true)
	}
	rows := make([]BodyRow, 0, len(order))
	for _, n := range order {
		rows = append(rows, *byName[n])
	}
	key := func(r BodyRow) float64 {
		if r.HasA {
			return r.After
		}
		return r.Before
	}
	slices.SortFunc(rows, func(x, y BodyRow) int {
		if (x.Type == "sun") != (y.Type == "sun") {
			if x.Type == "sun" {
				return -1
			}
			return 1
		}
		return cmp.Or(cmp.Compare(key(x), key(y)), cmp.Compare(x.Name, y.Name))
	})
	return rows
}
