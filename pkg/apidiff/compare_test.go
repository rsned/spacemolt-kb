package apidiff

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func step(t *testing.T, oldJS, newJS string) Step {
	t.Helper()
	return CompareStep(Snapshot{Date: "20260101", Schemas: schemas(t, oldJS)},
		Snapshot{Date: "20260102", Schemas: schemas(t, newJS)})
}

func diffFor(t *testing.T, s Step, display string) StructDiff {
	t.Helper()
	for _, d := range s.Diffs {
		if (d.New != nil && d.New.Display == display) || (d.New == nil && d.Old.Display == display) {
			return d
		}
	}
	t.Fatalf("no diff for %q in %d diffs", display, len(s.Diffs))
	return StructDiff{}
}

func change(t *testing.T, d StructDiff, name string) FieldChange {
	t.Helper()
	for _, c := range d.Fields {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no field change %q", name)
	return FieldChange{}
}

func TestCompareFieldAddRemove(t *testing.T) {
	s := step(t, `{"E":{"type":"object","properties":{"a":{"type":"string"}}}}`,
		`{"E":{"type":"object","properties":{"a":{"type":"string"},"flagship_bounty":{"type":"integer"}}}}`)
	if s.Counts != (Counts{Additions: 1, Structs: 1}) {
		t.Errorf("counts = %+v", s.Counts)
	}
	if got := s.Counts.Summary(); got != "1 addition across 1 struct." {
		t.Errorf("summary = %q", got)
	}
	if c := change(t, diffFor(t, s, "E"), "flagship_bounty"); c.Old != nil || c.New == nil {
		t.Errorf("flagship_bounty not an addition: %+v", c)
	}
}

func TestCompareEnumRemoved(t *testing.T) {
	s := step(t, `{"G":{"type":"object","properties":{"s":{"type":"string","enum":["a","b","c","d","e"]}}}}`,
		`{"G":{"type":"object","properties":{"s":{"type":"string","enum":["a"]}}}}`)
	if s.Counts.Removals != 4 {
		t.Errorf("removals = %d", s.Counts.Removals)
	}
	if got := change(t, diffFor(t, s, "G"), "s").EnumRemoved; !slices.Equal(got, []string{`"b"`, `"c"`, `"d"`, `"e"`}) {
		t.Errorf("enum removed = %v", got)
	}
}

func TestCompareEnumNonString(t *testing.T) {
	s := step(t, `{"G":{"type":"object","properties":{"n":{"enum":[1,2]}}}}`,
		`{"G":{"type":"object","properties":{"n":{"enum":[1,3]}}}}`)
	c := change(t, diffFor(t, s, "G"), "n")
	if !slices.Equal(c.EnumAdded, []string{"3"}) || !slices.Equal(c.EnumRemoved, []string{"2"}) {
		t.Errorf("enum = +%v -%v", c.EnumAdded, c.EnumRemoved)
	}
}

func TestCompareRetypeAndRequired(t *testing.T) {
	s := step(t, `{"P":{"type":"object","properties":{"a":{"type":"string"},"b":{"type":"string"}}}}`,
		`{"P":{"type":"object","required":["b"],"properties":{"a":{"type":"integer"},"b":{"type":"string"}}}}`)
	d := diffFor(t, s, "P")
	if !change(t, d, "a").Retyped || !change(t, d, "b").Retyped {
		t.Errorf("type and required changes should both be retypes")
	}
	if s.Counts.Changes != 2 {
		t.Errorf("changes = %d", s.Counts.Changes)
	}
}

func TestCompareInlineToRefNotRetype(t *testing.T) {
	s := step(t, `{"P":{"type":"object","properties":{"x":{"type":"object","properties":{"a":{"type":"string"},"b":{"type":"string"}}}}}}`,
		`{"P":{"type":"object","properties":{"x":{"$ref":"#/components/schemas/B"}}},
		  "B":{"type":"object","properties":{"a":{"type":"string"},"b":{"type":"string"}}}}`)
	if len(s.Diffs) != 0 {
		for _, d := range s.Diffs {
			t.Errorf("unexpected diff: old=%v new=%v", d.Old != nil, d.New != nil)
		}
	}
}

func TestCompareRenameNearMatch(t *testing.T) {
	s := step(t, `{"X":{"type":"object","properties":{"a":{},"b":{},"c":{},"d":{},"e":{}}}}`,
		`{"Y":{"type":"object","properties":{"a":{},"b":{},"c":{},"d":{},"e":{},"f":{}}}}`)
	if len(s.Diffs) != 1 || s.Diffs[0].Old == nil || s.Diffs[0].New == nil {
		t.Fatalf("want one matched pair, got %d diffs", len(s.Diffs))
	}
	if s.Counts.Additions != 1 {
		t.Errorf("additions = %d", s.Counts.Additions)
	}
}

func TestCompareVariantsByFieldsNotPosition(t *testing.T) {
	s := step(t, `{"P":{"oneOf":[{"type":"object","properties":{"a":{},"b":{}}},{"type":"object","properties":{"c":{},"d":{}}}]}}`,
		`{"P":{"oneOf":[{"type":"object","properties":{"c":{},"d":{}}},{"type":"object","properties":{"a":{},"b":{}}}]}}`)
	if len(s.Diffs) != 0 {
		t.Errorf("got %d diffs, want none", len(s.Diffs))
	}
}

func TestCompareUnionMembers(t *testing.T) {
	base := `"A":{"type":"object","properties":{"a":{}}},"C":{"type":"object","properties":{"c":{}}}`
	s := step(t, `{`+base+`,"U":{"oneOf":[{"$ref":"#/components/schemas/A"}]}}`,
		`{`+base+`,"U":{"oneOf":[{"$ref":"#/components/schemas/A"},{"$ref":"#/components/schemas/C"}]}}`)
	if s.Counts.Additions != 1 || !slices.Equal(diffFor(t, s, "U").MembersAdded, []string{"C"}) {
		t.Errorf("existing member join: counts %+v", s.Counts)
	}
	s = step(t, `{`+base+`,"U":{"oneOf":[{"$ref":"#/components/schemas/A"}]}}`,
		`{`+base+`,"D":{"type":"object","properties":{"zz":{}}},"U":{"oneOf":[{"$ref":"#/components/schemas/A"},{"$ref":"#/components/schemas/D"}]}}`)
	if s.Counts.Additions != 1 {
		t.Errorf("new struct joining should count once, got %+v", s.Counts)
	}
}

func TestCompareOrdering(t *testing.T) {
	s := step(t, `{"Z":{"type":"object","properties":{"a":{"description":"old"}}},"B":{"type":"object","properties":{"a":{}}},"A":{"type":"object","properties":{"a":{"description":"x"}}}}`,
		`{"Z":{"type":"object","properties":{"a":{"description":"new"}}},"B":{"type":"object","properties":{"a":{},"b":{}}},"A":{"type":"object","properties":{"a":{"description":"y"}}}}`)
	var got []string
	for _, d := range s.Diffs {
		got = append(got, d.New.Display)
	}
	if want := []string{"B", "A", "Z"}; !slices.Equal(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
	if s.Counts.Comments != 2 {
		t.Errorf("comments = %d", s.Counts.Comments)
	}
}

func TestCompareNestedNewCountsOnce(t *testing.T) {
	s := step(t, `{"P":{"type":"object","properties":{"a":{}}}}`,
		`{"P":{"type":"object","properties":{"a":{},"r":{"type":"object","properties":{"x":{},"y":{}}}}}}`)
	if s.Counts.Additions != 1 {
		t.Errorf("additions = %d, want 1", s.Counts.Additions)
	}
	diffFor(t, s, "P.r")
}

func TestCompareRefToNonStructUnchanged(t *testing.T) {
	js := `{"M":{"type":"object","additionalProperties":{"type":"string"}},
		"P":{"type":"object","properties":{"data":{"$ref":"#/components/schemas/M"}}}}`
	if s := step(t, js, js); s.Counts.Structs != 0 {
		t.Errorf("identical snapshots gave %d changed structs", s.Counts.Structs)
	}
}

func TestCompareExtractedDuplicatesMerge(t *testing.T) {
	s := step(t, `{"P":{"oneOf":[
		{"type":"object","properties":{"x":{},"summary":{"type":"object","properties":{"a":{},"b":{}}}}},
		{"type":"object","properties":{"y":{},"summary":{"type":"object","properties":{"a":{},"b":{}}}}}]}}`,
		`{"P":{"oneOf":[{"$ref":"#/components/schemas/X"},{"$ref":"#/components/schemas/Y"}]},
		  "X":{"type":"object","properties":{"x":{},"summary":{"$ref":"#/components/schemas/S"}}},
		  "Y":{"type":"object","properties":{"y":{},"summary":{"$ref":"#/components/schemas/S"}}},
		  "S":{"type":"object","properties":{"a":{},"b":{}}}}`)
	if s.Counts.Structs != 0 {
		for _, d := range s.Diffs {
			t.Errorf("unexpected diff %q old=%v new=%v", d.Display(), d.Old != nil, d.New != nil)
		}
	}
}

func TestCompareInlineMapToAliasRef(t *testing.T) {
	s := step(t, `{"P":{"type":"object","properties":{"data":{"type":"object","additionalProperties":{"type":"string"}}}}}`,
		`{"M":{"type":"object","additionalProperties":{"type":"string"}},
		  "P":{"type":"object","properties":{"data":{"$ref":"#/components/schemas/M"}}}}`)
	if s.Counts.Structs != 0 {
		t.Errorf("inline map → alias ref reported as %d changed structs", s.Counts.Structs)
	}
}

func TestCompareSameShapeRefNotRetype(t *testing.T) {
	s := step(t, `{"P":{"type":"object","properties":{"m":{"type":"array","items":{"type":"object","properties":{"item_id":{},"name":{},"quantity":{}}}}}}}`,
		`{"A":{"type":"object","properties":{"item_id":{},"name":{},"quantity":{}}},
		  "ItemQuantity":{"type":"object","properties":{"item_id":{},"name":{},"quantity":{}}},
		  "P":{"type":"object","properties":{"m":{"type":"array","items":{"$ref":"#/components/schemas/ItemQuantity"}}}}}`)
	for _, d := range s.Diffs {
		for _, f := range d.Fields {
			if f.Retyped {
				t.Errorf("%s.%s retyped", d.Display(), f.Name)
			}
		}
	}
}

func TestMatchOrderIsStable(t *testing.T) {
	var ov, nv []string
	for i := range 20 {
		ov = append(ov, fmt.Sprintf(`{"type":"object","properties":{"o%d":{}}}`, i))
		nv = append(nv, fmt.Sprintf(`{"type":"object","properties":{"n%d":{}}}`, i))
	}
	old := BuildModel(schemas(t, `{"X":{"oneOf":[`+strings.Join(ov, ",")+`]}}`))
	new := BuildModel(schemas(t, `{"X":{"oneOf":[`+strings.Join(nv, ",")+`]}}`))
	key := func(ps []Pair) string {
		s := ""
		for _, p := range ps {
			s += fmt.Sprintf("%v/%v;", p.Old != nil, p.New != nil)

		}
		return s
	}
	first := key(Match(old, new))
	for range 200 {
		if got := key(Match(old, new)); got != first {
			t.Fatalf("pair order changed between runs: %s vs %s", first, got)
		}
	}
}

func TestCompareVariantTiePrefersSameID(t *testing.T) {
	s := step(t, `{"B":{"oneOf":[{"type":"object","properties":{"p":{},"q":{}}}]},
		"A":{"type":"object","properties":{"s":{"type":"array","items":{"type":"object","properties":{"p":{},"q":{}}}}}}}`,
		`{"B":{"oneOf":[{"type":"object","properties":{"p":{},"q":{}}}]},"Z":{"type":"object","properties":{"p":{},"q":{}}},
		"A":{"type":"object","properties":{"s":{"type":"array","items":{"$ref":"#/components/schemas/Z"}}}}}`)
	if s.Counts.Structs != 0 {
		for _, d := range s.Diffs {
			t.Errorf("spurious diff %q old=%v new=%v m+%v m-%v", d.Display(), d.Old != nil, d.New != nil, d.MembersAdded, d.MembersRemoved)
		}
	}
}

func TestCompareVariantGainingFieldsShowsFields(t *testing.T) {
	s := step(t, `{"X":{"oneOf":[{"type":"object","properties":{"a":{},"b":{},"c":{},"d":{}}}]}}`,
		`{"X":{"oneOf":[{"type":"object","properties":{"a":{},"b":{},"c":{},"d":{},"e":{},"f":{}}}]}}`)
	if len(s.Diffs) != 1 || s.Diffs[0].Old == nil || s.Diffs[0].New == nil {
		t.Fatalf("want one matched variant, got %d diffs", len(s.Diffs))
	}
	if s.Counts.Additions != 2 || len(s.Diffs[0].MembersAdded)+len(s.Diffs[0].MembersRemoved) != 0 {
		t.Errorf("counts %+v", s.Counts)
	}
}

func TestComparePositionalMembersNoUnionEntry(t *testing.T) {
	s := step(t, `{"X":{"oneOf":[{"type":"object","properties":{"a":{}}}]}}`,
		`{"X":{"oneOf":[{"type":"object","properties":{"a":{}}},{"type":"object","properties":{"z":{}}}]}}`)
	for _, d := range s.Diffs {
		if len(d.MembersAdded)+len(d.MembersRemoved) > 0 {
			t.Errorf("%s lists positional members %v %v", d.Display(), d.MembersAdded, d.MembersRemoved)
		}
	}
}
