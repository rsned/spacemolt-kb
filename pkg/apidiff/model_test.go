package apidiff

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// schemas decodes a components.schemas literal the way LoadSnapshots does.
func schemas(t *testing.T, js string) map[string]any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(js))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatal(err)
	}
	return m
}

func field(t *testing.T, s *Struct, name string) Field {
	t.Helper()
	if s == nil {
		t.Fatalf("struct missing")
	}
	for _, f := range s.Fields {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("%s: no field %q", s.ID, name)
	return Field{}
}

func TestBuildModelNullable(t *testing.T) {
	m := BuildModel(schemas(t, `{"P":{"type":"object","properties":{
		"a":{"type":"string","nullable":true},
		"b":{"type":["string","null"]},
		"c":{"type":["array","null"],"items":{"type":"integer"}}}}}`))
	for _, n := range []string{"a", "b"} {
		f := field(t, m["P"], n)
		if f.Type.Kind != KindScalar || f.Type.Scalar != "string" || !f.Type.Nullable {
			t.Errorf("%s = %+v, want nullable string", n, f.Type)
		}
	}
	c := field(t, m["P"], "c").Type
	if c.Kind != KindArray || !c.Nullable || c.Elem == nil || c.Elem.Scalar != "integer" {
		t.Errorf("c = %+v, want nullable []integer", c)
	}
}

func TestBuildModelBoolSchema(t *testing.T) {
	m := BuildModel(schemas(t, `{"P":{"type":"object","properties":{"x":true}}}`))
	if k := field(t, m["P"], "x").Type.Kind; k != KindAny {
		t.Errorf("kind = %v, want KindAny", k)
	}
}

func TestBuildModelNested(t *testing.T) {
	m := BuildModel(schemas(t, `{"P":{"type":"object","properties":{
		"rewards":{"type":"object","properties":{"pirate_rep":{"type":"integer"}}},
		"list":{"type":"array","items":{"type":"object","properties":{"id":{"type":"string"}}}}}}}`))
	r := field(t, m["P"], "rewards").Type
	if r.Kind != KindNested || r.Ref != "P.rewards" {
		t.Errorf("rewards = %+v", r)
	}
	field(t, m["P.rewards"], "pirate_rep")
	l := field(t, m["P"], "list").Type
	if l.Kind != KindArray || l.Elem == nil || l.Elem.Kind != KindNested || l.Elem.Ref != "P.list[]" {
		t.Errorf("list = %+v", l)
	}
	field(t, m["P.list[]"], "id")
}

func TestBuildModelVariants(t *testing.T) {
	m := BuildModel(schemas(t, `{
		"A":{"type":"object","properties":{"a":{"type":"string"}}},
		"P":{"oneOf":[{"$ref":"#/components/schemas/A"},
			{"type":"object","properties":{"action":{"type":"string","enum":["list"]},"b":{"type":"string"}}},
			{"type":"object","properties":{"c":{"type":"string"}}}]}}`))
	if got, want := m["P"].Members, []string{"A", "P#v1", "P#v2"}; !slices.Equal(got, want) {
		t.Errorf("members = %v, want %v", got, want)
	}
	if d := m["P#v1"].Display; d != "P (list)" {
		t.Errorf("display = %q", d)
	}
	if d := m["P#v2"].Display; d != "P (variant 2)" {
		t.Errorf("display = %q", d)
	}
	m = BuildModel(schemas(t, `{"P":{"oneOf":[{"type":"object","properties":{"action":{"enum":["go"]},"x":{"type":"object","properties":{"y":{"type":"string"}}}}}]}}`))
	if d := m["P#v0.x"].Display; d != "P (go).x" {
		t.Errorf("display = %q", d)
	}
}

func TestBuildModelPropsAndUnion(t *testing.T) {
	m := BuildModel(schemas(t, `{"A":{"type":"object","properties":{"a":{"type":"string"}}},
		"P":{"type":"object","properties":{"x":{"type":"string"}},"anyOf":[{"$ref":"#/components/schemas/A"}]}}`))
	field(t, m["P"], "x")
	if !slices.Equal(m["P"].Members, []string{"A"}) {
		t.Errorf("members = %v", m["P"].Members)
	}
}

func TestBuildModelEnumLiterals(t *testing.T) {
	m := BuildModel(schemas(t, `{"P":{"type":"object","properties":{
		"s":{"type":"string","enum":["a"]},"n":{"enum":[1,true]}}}}`))
	if got := field(t, m["P"], "s").Enum; !slices.Equal(got, []string{`"a"`}) {
		t.Errorf("s enum = %v", got)
	}
	if got := field(t, m["P"], "n").Enum; !slices.Equal(got, []string{"1", "true"}) {
		t.Errorf("n enum = %v", got)
	}
}

func TestBuildModelRequiredAndMapOnly(t *testing.T) {
	m := BuildModel(schemas(t, `{"P":{"type":"object","required":["a"],"properties":{"a":{"type":"string"},"b":{"type":"string"}}},
		"M":{"type":"object","additionalProperties":{"type":"integer"}}}`))
	if !field(t, m["P"], "a").Required || field(t, m["P"], "b").Required {
		t.Errorf("required flags wrong")
	}
	if _, ok := m["M"]; ok {
		t.Errorf("map-only schema should not be a struct")
	}
}
