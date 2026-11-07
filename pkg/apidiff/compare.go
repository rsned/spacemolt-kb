package apidiff

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// FieldChange is one field across a pair. Old or New is nil for an added or
// removed field; unchanged fields are included so renderers can fold them.
type FieldChange struct {
	Name                   string
	Old, New               *Field
	Retyped, Reword        bool
	EnumAdded, EnumRemoved []string
	enumFlip               bool // the field gained or lost its enum: counted once, as a retype
}

// Changed reports whether anything about the field changed.
func (c FieldChange) Changed() bool {
	return c.Old == nil || c.New == nil || c.Retyped || c.Reword || len(c.EnumAdded) > 0 || len(c.EnumRemoved) > 0
}

// StructDiff is a changed struct.
type StructDiff struct {
	Pair
	Fields                       []FieldChange // every field, in render order
	MembersAdded, MembersRemoved []string      // display names
	MembersKept                  []string      // display names, for context
	DescChanged                  bool
}

// Structural reports whether anything other than wording changed.
func (d StructDiff) Structural() bool {
	if d.Old == nil || d.New == nil || len(d.MembersAdded) > 0 || len(d.MembersRemoved) > 0 {
		return true
	}
	for _, c := range d.Fields {
		if c.Old == nil || c.New == nil || c.Retyped || len(c.EnumAdded) > 0 || len(c.EnumRemoved) > 0 {
			return true
		}
	}
	return false
}

func (d StructDiff) changed() bool {
	if d.Structural() || d.DescChanged {
		return true
	}
	return slices.ContainsFunc(d.Fields, FieldChange.Changed)
}

// Display is the card title.
func (d StructDiff) Display() string {
	if d.New != nil {
		return d.New.Display
	}
	return d.Old.Display
}

// Counts summarise a step.
type Counts struct{ Additions, Removals, Changes, Comments, Structs int }

// Step is the comparison of two adjacent snapshots.
type Step struct {
	From, To string       // YYYYMMDD
	Diffs    []StructDiff // structural first, then wording; each by display name
	Counts   Counts
}

// CompareStep compares two snapshots.
func CompareStep(from, to Snapshot) Step {
	om, nm := BuildModel(from.Schemas), BuildModel(to.Schemas)
	pairs := Match(om, nm)
	// oldToNew maps an old struct ID to the ID of the new struct it became.
	// An old struct with no match maps to a marker that equals nothing new;
	// a ref to a schema that never became a struct (a map-only schema) maps
	// to itself.
	oldToNew := map[string]string{}
	newMatched := map[string]bool{}
	for _, p := range pairs {
		switch {
		case p.Old != nil && p.New != nil:
			oldToNew[p.Old.ID] = p.New.ID
			newMatched[p.New.ID] = true
		case p.Old != nil:
			oldToNew[p.Old.ID] = "old:" + p.Old.ID
		}
	}
	eq := typeEq{om: om, nm: nm, oldToNew: oldToNew}
	st := Step{From: from.Date, To: to.Date}
	for _, p := range pairs {
		if p.Merged {
			continue
		}
		d := StructDiff{Pair: p}
		c := &st.Counts
		nested := strings.Contains(pairKey(p), ".")
		switch {
		case p.Old == nil:
			d.Fields = fieldChanges(nil, p.New.Fields, eq)
			if !nested {
				c.Additions++
			}
		case p.New == nil:
			d.Fields = fieldChanges(p.Old.Fields, nil, eq)
			if !nested {
				c.Removals++
			}
		default:
			d.Fields = fieldChanges(p.Old.Fields, p.New.Fields, eq)
			d.DescChanged = norm(p.Old.Desc) != norm(p.New.Desc)
			if d.DescChanged {
				c.Comments++
			}
			for _, f := range d.Fields {
				switch {
				case f.Old == nil:
					c.Additions++
				case f.New == nil:
					c.Removals++
				default:
					if f.Retyped {
						c.Changes++
					}
					if f.Reword {
						c.Comments++
					}
					if !f.enumFlip {
						c.Additions += len(f.EnumAdded)
						c.Removals += len(f.EnumRemoved)
					}
				}
			}
			oldSet := map[string]bool{}
			for _, m := range p.Old.Members {
				oldSet[mapID(m, oldToNew)] = true
			}
			newSet := map[string]bool{}
			for _, m := range p.New.Members {
				newSet[m] = true
				if oldSet[m] {
					d.MembersKept = append(d.MembersKept, displayOf(nm, m))
					continue
				}
				if positional(m) {
					continue // an inline variant is compared as a struct, not a union entry
				}
				d.MembersAdded = append(d.MembersAdded, displayOf(nm, m))
				if newMatched[m] || nm[m] == nil {
					c.Additions++
				}
			}
			for _, m := range p.Old.Members {
				id := mapID(m, oldToNew)
				if newSet[id] || positional(m) {
					continue
				}
				d.MembersRemoved = append(d.MembersRemoved, displayOf(om, m))
				if n, ok := oldToNew[m]; (ok && !strings.HasPrefix(n, "old:")) || om[m] == nil {
					c.Removals++
				}
			}
		}
		if d.changed() {
			st.Diffs = append(st.Diffs, d)
			c.Structs++
		}
	}
	sort.SliceStable(st.Diffs, func(i, j int) bool {
		a, b := st.Diffs[i], st.Diffs[j]
		if a.Structural() != b.Structural() {
			return a.Structural()
		}
		if a.Display() != b.Display() {
			return a.Display() < b.Display()
		}
		return pairKey(a.Pair)+pairRank(a.Pair) < pairKey(b.Pair)+pairRank(b.Pair)
	})
	return st
}

func fieldChanges(old, new []Field, eq typeEq) []FieldChange {
	byName := func(fs []Field) map[string]*Field {
		m := map[string]*Field{}
		for i := range fs {
			m[fs[i].Name] = &fs[i]
		}
		return m
	}
	om, nm := byName(old), byName(new)
	names := map[string]bool{}
	for k := range om {
		names[k] = true
	}
	for k := range nm {
		names[k] = true
	}
	sorted := make([]string, 0, len(names))
	for k := range names {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	out := make([]FieldChange, 0, len(sorted))
	for _, k := range sorted {
		fc := FieldChange{Name: k, Old: om[k], New: nm[k]}
		if fc.Old != nil && fc.New != nil {
			fc.enumFlip = (len(fc.Old.Enum) > 0) != (len(fc.New.Enum) > 0)
			fc.Retyped = fc.Old.Required != fc.New.Required || fc.enumFlip || !eq.equal(fc.Old.Type, fc.New.Type)
			fc.Reword = norm(fc.Old.Desc) != norm(fc.New.Desc)
			fc.EnumAdded = minus(fc.New.Enum, fc.Old.Enum)
			fc.EnumRemoved = minus(fc.Old.Enum, fc.New.Enum)
		}
		out = append(out, fc)
	}
	return out
}

// typeEq compares an old field type with a new one.
type typeEq struct {
	om, nm   map[string]*Struct
	oldToNew map[string]string
}

// equal treats two struct-typed fields as the same type when the old struct
// became the new one, or when their field sets match at nearMatch: a shape
// such as {item_id, name, quantity} is often extracted under several names.
func (e typeEq) equal(a, b TypeRef) bool {
	if a.Nullable != b.Nullable {
		return false
	}
	structy := func(k TypeKind) bool { return k == KindRef || k == KindNested }
	switch {
	case structy(a.Kind) && structy(b.Kind):
		if mapID(a.Ref, e.oldToNew) == b.Ref {
			return true
		}
		o, n := e.om[a.Ref], e.nm[b.Ref]
		return o != nil && n != nil && jaccard(o, n) >= nearMatch
	case a.Kind != b.Kind:
		return false
	case a.Kind == KindArray || a.Kind == KindMap:
		if a.Elem == nil || b.Elem == nil {
			return a.Elem == b.Elem
		}
		return e.equal(*a.Elem, *b.Elem)
	}
	return typeKey(a, nil) == typeKey(b, nil)
}

// typeKey renders a type canonically. Nested and ref types both resolve to the
// struct identity, so an inline object that became a $ref to a matched struct
// compares equal.
func typeKey(t TypeRef, oldToNew map[string]string) string {
	n := ""
	if t.Nullable {
		n = "?"
	}
	switch t.Kind {
	case KindScalar:
		return t.Scalar + n
	case KindArray, KindMap:
		e := "any"
		if t.Elem != nil {
			e = typeKey(*t.Elem, oldToNew)
		}
		return fmt.Sprintf("%d[%s]%s", t.Kind, e, n)
	case KindRef, KindNested:
		return "@" + mapID(t.Ref, oldToNew) + n
	}
	return "any" + n
}

func mapID(id string, oldToNew map[string]string) string {
	if oldToNew == nil {
		return id
	}
	if n, ok := oldToNew[id]; ok {
		return n
	}
	return id
}

func displayOf(m map[string]*Struct, id string) string {
	if s := m[id]; s != nil {
		return s.Display
	}
	return id
}

func minus(a, b []string) []string {
	var out []string
	for _, x := range a {
		if !slices.Contains(b, x) {
			out = append(out, x)
		}
	}
	return out
}

func norm(s string) string { return strings.Join(strings.Fields(s), " ") }

// Summary is the one-line step summary, or "" when nothing changed.
func (c Counts) Summary() string {
	if c.Structs == 0 {
		return ""
	}
	var parts []string
	add := func(n int, one, many string) {
		if n == 1 {
			parts = append(parts, "1 "+one)
		} else if n > 1 {
			parts = append(parts, fmt.Sprintf("%d %s", n, many))
		}
	}
	add(c.Additions, "addition", "additions")
	add(c.Removals, "removal", "removals")
	add(c.Changes, "change", "changes")
	add(c.Comments, "comment", "comments")
	s := "1 struct"
	if c.Structs != 1 {
		s = fmt.Sprintf("%d structs", c.Structs)
	}
	if len(parts) == 0 {
		return "Changes across " + s + "."
	}
	return strings.Join(parts, ", ") + " across " + s + "."
}
