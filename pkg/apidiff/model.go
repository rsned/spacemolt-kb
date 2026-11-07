package apidiff

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// TypeKind classifies a field's type.
type TypeKind int

// Type kinds.
const (
	KindAny TypeKind = iota
	KindScalar
	KindArray
	KindMap
	KindRef
	KindNested
)

// TypeRef is a normalised field type. 3.0.3 nullable:true and 3.1 type lists
// both set Nullable.
type TypeRef struct {
	Kind     TypeKind
	Scalar   string   // "integer" | "number" | "boolean" | "string"
	Elem     *TypeRef // KindArray, KindMap
	Ref      string   // KindRef: schema name; KindNested: the nested Struct's ID
	Nullable bool
}

// Field is one property of a struct.
type Field struct {
	Name     string
	Type     TypeRef
	Required bool
	Desc     string
	Enum     []string // literal form: strings quoted, numbers and bools bare
}

// Struct is a named schema, an inline union variant or a nested inline object.
type Struct struct {
	ID      string // "Name", "Parent.field", "Parent.field[]", "Parent#v<idx>"
	Display string // card title
	Desc    string
	Fields  []Field  // sorted by name: the specs list properties alphabetically
	Members []string // oneOf/anyOf member IDs, in spec order
}

// BuildModel turns components.schemas into structs keyed by ID. Schemas with
// neither properties nor union members (map-only schemas) produce none.
func BuildModel(schemas map[string]any) map[string]*Struct {
	b := builder{schemas: schemas, out: map[string]*Struct{}}
	for name, n := range schemas {
		if m, ok := n.(map[string]any); ok {
			b.object(name, name, m)
		}
	}
	return b.out
}

type builder struct {
	schemas map[string]any
	out     map[string]*Struct
}

// object records node as struct id when it has properties or members, and
// reports whether it did.
func (b *builder) object(id, display string, node map[string]any) bool {
	props := map[string]any{}
	var required []string
	b.collect(node, props, &required, 0)
	var members []string
	for _, key := range []string{"oneOf", "anyOf"} {
		list, _ := node[key].([]any)
		for i, x := range list {
			xm, ok := x.(map[string]any)
			if !ok {
				continue
			}
			if r := refName(xm); r != "" {
				members = append(members, r)
				continue
			}
			vid := fmt.Sprintf("%s#v%d", id, i)
			if b.object(vid, variantDisplay(display, i, xm), xm) {
				members = append(members, vid)
			}
		}
	}
	if len(props) == 0 && len(members) == 0 {
		return false
	}
	s := &Struct{ID: id, Display: display, Desc: str(node["description"]), Members: members}
	req := map[string]bool{}
	for _, r := range required {
		req[r] = true
	}
	names := make([]string, 0, len(props))
	for k := range props {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		f := Field{Name: k, Required: req[k]}
		f.Type = b.typeOf(props[k], id+"."+k, display+"."+k)
		if pm, ok := props[k].(map[string]any); ok {
			f.Desc = str(pm["description"])
			f.Enum = enumLiterals(pm["enum"])
		}
		s.Fields = append(s.Fields, f)
	}
	b.out[id] = s
	return true
}

// collect gathers properties and required names, merging allOf parts.
func (b *builder) collect(node map[string]any, props map[string]any, required *[]string, depth int) {
	if depth > 8 {
		return
	}
	if p, ok := node["properties"].(map[string]any); ok {
		for k, v := range p {
			props[k] = v
		}
	}
	if r, ok := node["required"].([]any); ok {
		for _, x := range r {
			*required = append(*required, str(x))
		}
	}
	all, _ := node["allOf"].([]any)
	for _, x := range all {
		xm, ok := x.(map[string]any)
		if !ok {
			continue
		}
		if r := refName(xm); r != "" {
			if rm, ok := b.schemas[r].(map[string]any); ok {
				b.collect(rm, props, required, depth+1)
			}
			continue
		}
		b.collect(xm, props, required, depth+1)
	}
}

// typeOf normalises a property schema; id and display name a nested struct
// if one is needed.
func (b *builder) typeOf(n any, id, display string) TypeRef {
	m, ok := n.(map[string]any)
	if !ok {
		return TypeRef{Kind: KindAny}
	}
	if r := refName(m); r != "" {
		return TypeRef{Kind: KindRef, Ref: r, Nullable: m["nullable"] == true}
	}
	t, nullable := baseType(m)
	var tr TypeRef
	switch {
	case t == "array":
		e := b.typeOf(m["items"], id+"[]", display+"[]")
		tr = TypeRef{Kind: KindArray, Elem: &e}
	case b.object(id, display, m):
		tr = TypeRef{Kind: KindNested, Ref: id}
	case t == "object":
		e := TypeRef{Kind: KindAny}
		if ap, ok := m["additionalProperties"].(map[string]any); ok {
			e = b.typeOf(ap, id+"{}", display+"{}")
		}
		tr = TypeRef{Kind: KindMap, Elem: &e}
	case t == "integer" || t == "number" || t == "boolean" || t == "string":
		tr = TypeRef{Kind: KindScalar, Scalar: t}
	default:
		tr = TypeRef{Kind: KindAny}
	}
	tr.Nullable = nullable
	return tr
}

func baseType(m map[string]any) (string, bool) {
	nullable := m["nullable"] == true
	switch t := m["type"].(type) {
	case string:
		return t, nullable
	case []any:
		base := ""
		for _, x := range t {
			if s := str(x); s == "null" {
				nullable = true
			} else if base == "" {
				base = s
			}
		}
		return base, nullable
	}
	return "", nullable
}

func refName(m map[string]any) string {
	r, _ := m["$ref"].(string)
	if i := strings.LastIndex(r, "/"); i >= 0 {
		return r[i+1:]
	}
	return r
}

func variantDisplay(parent string, i int, m map[string]any) string {
	if p, ok := m["properties"].(map[string]any); ok {
		if a, ok := p["action"].(map[string]any); ok {
			if c := str(a["const"]); c != "" {
				return parent + " (" + c + ")"
			}
			if e, ok := a["enum"].([]any); ok && len(e) == 1 {
				return parent + " (" + str(e[0]) + ")"
			}
		}
	}
	return fmt.Sprintf("%s (variant %d)", parent, i)
}

func enumLiterals(v any) []string {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, x := range list {
		switch t := x.(type) {
		case string:
			out = append(out, strconv.Quote(t))
		default:
			out = append(out, str(t))
		}
	}
	return out
}

func str(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case json.Number:
		return t.String()
	case bool:
		return strconv.FormatBool(t)
	}
	return fmt.Sprint(v)
}
