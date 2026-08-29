package resourcediff

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// TypeChange is a resource type whose discovery state changed.
type TypeChange struct {
	ResourceType
	Deposits int // deposit count in the newer snapshot
}

// DepositGroup is the new deposits of one resource.
type DepositGroup struct {
	ResourceType
	Rows []Deposit
}

// DepositRow is a deposit annotated with its resource's display name.
type DepositRow struct {
	Deposit
	ResourceName string
}

// DepositChange is a deposit whose surveyed values changed.
type DepositChange struct {
	Old, New     Deposit
	ResourceName string
}

// SystemRef is a system that first gained a deposit.
type SystemRef struct {
	ID, Name string
	Deposits int
}

// POIRef is a POI that first gained a deposit.
type POIRef struct {
	ID, Name             string
	SystemID, SystemName string
	Hidden               bool
	Deposits             int
}

// Comparison is the difference between an older and a newer snapshot.
type Comparison struct {
	OldDate, NewDate string
	OldVersion       string
	Old, New         Summary

	NewTypes     []ResourceType // in the catalog now, absent before
	RemovedTypes []ResourceType
	Discovered   []TypeChange // had no deposits before, has some now
	Lost         []TypeChange // had deposits before, has none now

	NewDeposits     []DepositGroup // grouped by resource, sorted by resource name
	RemovedDeposits []DepositRow
	Changed         []DepositChange // richness, remaining, or hidden changed

	NewSystems []SystemRef // systems with their first surveyed deposit
	NewPOIs    []POIRef    // POIs with their first surveyed deposit
}

// Diff compares two snapshots.
func Diff(old, cur *Snapshot) *Comparison {
	c := &Comparison{
		OldDate: old.Date, NewDate: cur.Date, OldVersion: old.ServerVersion,
		Old: old.Summary, New: cur.Summary,
	}

	oldTypes := indexTypes(old.Types)
	newTypes := indexTypes(cur.Types)
	for _, t := range cur.Types {
		if _, ok := oldTypes[t.ID]; !ok {
			c.NewTypes = append(c.NewTypes, t)
		}
	}
	for _, t := range old.Types {
		if _, ok := newTypes[t.ID]; !ok {
			c.RemovedTypes = append(c.RemovedTypes, t)
		}
	}

	oldByKey := make(map[string]Deposit, len(old.Deposits))
	oldPerType := make(map[string]int)
	oldSystems := make(map[string]bool)
	oldPOIs := make(map[string]bool)
	for _, d := range old.Deposits {
		oldByKey[d.Key()] = d
		oldPerType[d.ResourceID]++
		oldSystems[d.SystemID] = true
		oldPOIs[d.POIID] = true
	}
	newByKey := make(map[string]Deposit, len(cur.Deposits))
	newPerType := make(map[string]int)
	for _, d := range cur.Deposits {
		newByKey[d.Key()] = d
		newPerType[d.ResourceID]++
	}

	typeOf := func(id string) ResourceType {
		if t, ok := newTypes[id]; ok {
			return t
		}
		if t, ok := oldTypes[id]; ok {
			return t
		}
		return ResourceType{ID: id, Name: id}
	}
	for id, n := range newPerType {
		if oldPerType[id] == 0 {
			c.Discovered = append(c.Discovered, TypeChange{ResourceType: typeOf(id), Deposits: n})
		}
	}
	for id := range oldPerType {
		if newPerType[id] == 0 {
			c.Lost = append(c.Lost, TypeChange{ResourceType: typeOf(id), Deposits: 0})
		}
	}

	groups := make(map[string]*DepositGroup)
	newSystems := make(map[string]*SystemRef)
	newPOIs := make(map[string]*POIRef)
	for _, d := range cur.Deposits {
		o, existed := oldByKey[d.Key()]
		if existed {
			if o.Richness != d.Richness || o.Remaining != d.Remaining || o.Hidden != d.Hidden {
				c.Changed = append(c.Changed, DepositChange{Old: o, New: d, ResourceName: typeOf(d.ResourceID).Name})
			}
			continue
		}
		g, ok := groups[d.ResourceID]
		if !ok {
			g = &DepositGroup{ResourceType: typeOf(d.ResourceID)}
			groups[d.ResourceID] = g
		}
		g.Rows = append(g.Rows, d)
		if !oldSystems[d.SystemID] {
			s, ok := newSystems[d.SystemID]
			if !ok {
				s = &SystemRef{ID: d.SystemID, Name: d.SystemName}
				newSystems[d.SystemID] = s
			}
			s.Deposits++
		}
		if !oldPOIs[d.POIID] {
			p, ok := newPOIs[d.POIID]
			if !ok {
				p = &POIRef{ID: d.POIID, Name: d.POIName, SystemID: d.SystemID, SystemName: d.SystemName, Hidden: d.Hidden}
				newPOIs[d.POIID] = p
			}
			p.Deposits++
		}
	}
	for _, d := range old.Deposits {
		if _, ok := newByKey[d.Key()]; !ok {
			c.RemovedDeposits = append(c.RemovedDeposits, DepositRow{Deposit: d, ResourceName: typeOf(d.ResourceID).Name})
		}
	}

	for _, g := range groups {
		c.NewDeposits = append(c.NewDeposits, *g)
	}
	for _, s := range newSystems {
		c.NewSystems = append(c.NewSystems, *s)
	}
	for _, p := range newPOIs {
		c.NewPOIs = append(c.NewPOIs, *p)
	}

	byName := func(a, b ResourceType) int { return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.ID, b.ID)) }
	slices.SortFunc(c.NewTypes, byName)
	slices.SortFunc(c.RemovedTypes, byName)
	slices.SortFunc(c.Discovered, func(a, b TypeChange) int { return byName(a.ResourceType, b.ResourceType) })
	slices.SortFunc(c.Lost, func(a, b TypeChange) int { return byName(a.ResourceType, b.ResourceType) })
	slices.SortFunc(c.NewDeposits, func(a, b DepositGroup) int { return byName(a.ResourceType, b.ResourceType) })
	slices.SortFunc(c.RemovedDeposits, func(a, b DepositRow) int {
		return cmp.Or(cmp.Compare(a.ResourceName, b.ResourceName), cmp.Compare(a.SystemName, b.SystemName), cmp.Compare(a.POIName, b.POIName))
	})
	slices.SortFunc(c.Changed, func(a, b DepositChange) int {
		return cmp.Or(cmp.Compare(a.ResourceName, b.ResourceName), cmp.Compare(a.New.SystemName, b.New.SystemName), cmp.Compare(a.New.POIName, b.New.POIName))
	})
	slices.SortFunc(c.NewSystems, func(a, b SystemRef) int { return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.ID, b.ID)) })
	slices.SortFunc(c.NewPOIs, func(a, b POIRef) int {
		return cmp.Or(cmp.Compare(a.SystemName, b.SystemName), cmp.Compare(a.Name, b.Name), cmp.Compare(a.ID, b.ID))
	})
	return c
}

func indexTypes(types []ResourceType) map[string]ResourceType {
	m := make(map[string]ResourceType, len(types))
	for _, t := range types {
		m[t.ID] = t
	}
	return m
}

// HasChanges reports whether anything differs between the two snapshots.
func (c *Comparison) HasChanges() bool {
	return len(c.NewTypes)+len(c.RemovedTypes)+len(c.Discovered)+len(c.Lost)+
		len(c.NewDeposits)+len(c.RemovedDeposits)+len(c.Changed) > 0
}

// NewDepositCount is the total number of new deposit rows across all groups.
func (c *Comparison) NewDepositCount() int {
	n := 0
	for _, g := range c.NewDeposits {
		n += len(g.Rows)
	}
	return n
}

// NewHiddenPOIs is the number of newly surveyed POIs flagged hidden.
func (c *Comparison) NewHiddenPOIs() int {
	n := 0
	for _, p := range c.NewPOIs {
		if p.Hidden {
			n++
		}
	}
	return n
}

// SummaryLine is a compact description for index pages, e.g.
// "2 new types, 1 discovered, 14 new deposits, 5 new POIs (2 hidden)".
func (c *Comparison) SummaryLine() string {
	if !c.HasChanges() {
		return "No changes"
	}
	var parts []string
	add := func(n int, singular string) {
		if n == 0 {
			return
		}
		s := singular
		if n != 1 {
			s += "s"
		}
		parts = append(parts, fmt.Sprintf("%d %s", n, s))
	}
	add(len(c.NewTypes), "new type")
	add(len(c.RemovedTypes), "removed type")
	if n := len(c.Discovered); n > 0 {
		parts = append(parts, fmt.Sprintf("%d discovered", n))
	}
	if n := len(c.Lost); n > 0 {
		parts = append(parts, fmt.Sprintf("%d lost", n))
	}
	add(c.NewDepositCount(), "new deposit")
	add(len(c.RemovedDeposits), "removed deposit")
	if n := len(c.NewPOIs); n > 0 {
		s := fmt.Sprintf("%d new POI", n)
		if n != 1 {
			s += "s"
		}
		if h := c.NewHiddenPOIs(); h > 0 {
			s += fmt.Sprintf(" (%d hidden)", h)
		}
		parts = append(parts, s)
	}
	add(len(c.NewSystems), "new system")
	add(len(c.Changed), "re-surveyed deposit")
	return strings.Join(parts, ", ")
}

// ResourceDelta summarises one resource's deposit movement in a comparison.
type ResourceDelta struct {
	Added      int  // new deposit rows
	Removed    int  // deposit rows no longer listed
	Discovered bool // had no deposits in the older snapshot, has some now
	NewType    bool // absent from the older snapshot's catalog
}

// Net is added minus removed.
func (d ResourceDelta) Net() int { return d.Added - d.Removed }

// PerResource indexes the comparison's deposit movement by resource ID.
// Resources with no movement are absent.
func (c *Comparison) PerResource() map[string]ResourceDelta {
	out := make(map[string]ResourceDelta)
	for _, g := range c.NewDeposits {
		d := out[g.ID]
		d.Added += len(g.Rows)
		out[g.ID] = d
	}
	for _, r := range c.RemovedDeposits {
		d := out[r.ResourceID]
		d.Removed++
		out[r.ResourceID] = d
	}
	for _, t := range c.Discovered {
		d := out[t.ID]
		d.Discovered = true
		out[t.ID] = d
	}
	for _, t := range c.NewTypes {
		d := out[t.ID]
		d.NewType = true
		out[t.ID] = d
	}
	return out
}
