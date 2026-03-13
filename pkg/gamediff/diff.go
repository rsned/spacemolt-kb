// Package gamediff compares game API JSON snapshots and reports structural changes.
package gamediff

import (
	"encoding/json"
	"fmt"
	"slices"
)

// Entry is a named item that was added or removed.
type Entry struct {
	Name string
	ID   string
}

// Change is a field-level modification on a single item.
type Change struct {
	ID     string
	Field  string
	OldVal string // JSON-encoded
	NewVal string // JSON-encoded
}

// CatalogDiff holds the result of comparing two snapshots of a single catalog.
type CatalogDiff struct {
	Name      string // display name, e.g. "Recipes"
	File      string // filename, e.g. "catalog_recipes.json"
	Additions []Entry
	Deletions []Entry
	Changes   []Change
}

// HasChanges reports whether this catalog has any differences.
func (d *CatalogDiff) HasChanges() bool {
	return len(d.Additions) > 0 || len(d.Deletions) > 0 || len(d.Changes) > 0
}

// DiffCatalog compares two catalog JSON blobs (top-level {"items":[...]}).
func DiffCatalog(oldData, newData []byte) (*CatalogDiff, error) {
	oldItems, err := extractArray(oldData, "items", "id")
	if err != nil {
		return nil, fmt.Errorf("old: %w", err)
	}
	newItems, err := extractArray(newData, "items", "id")
	if err != nil {
		return nil, fmt.Errorf("new: %w", err)
	}
	return diffMaps(oldItems, newItems, "name", nil), nil
}

// mapIgnoreFields lists fields to skip when diffing map data (e.g. "online"
// changes on every snapshot and is not interesting for structural diffs).
var mapIgnoreFields = map[string]bool{
	"online":     true,
	"visited":    true,
	"visited_at": true,
}

// DiffMap compares two map JSON blobs (top-level {"systems":[...]}).
func DiffMap(oldData, newData []byte) (*CatalogDiff, error) {
	oldSystems, err := extractArray(oldData, "systems", "system_id")
	if err != nil {
		return nil, fmt.Errorf("old: %w", err)
	}
	newSystems, err := extractArray(newData, "systems", "system_id")
	if err != nil {
		return nil, fmt.Errorf("new: %w", err)
	}
	return diffMaps(oldSystems, newSystems, "name", mapIgnoreFields), nil
}

// extractArray parses JSON, pulls out the named top-level array, and indexes
// each element by the given key field.
func extractArray(data []byte, arrayField, keyField string) (map[string]map[string]any, error) {
	var top map[string]any
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, err
	}
	arr, ok := top[arrayField].([]any)
	if !ok {
		return nil, fmt.Errorf("field %q is not an array", arrayField)
	}
	result := make(map[string]map[string]any, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id, ok := m[keyField].(string)
		if !ok {
			continue
		}
		result[id] = m
	}
	return result, nil
}

// diffMaps compares two indexed maps and returns additions, deletions, and
// field-level changes. nameField is used for display names.
func diffMaps(oldMap, newMap map[string]map[string]any, nameField string, ignoreFields map[string]bool) *CatalogDiff {
	result := &CatalogDiff{}

	for id, oldItem := range oldMap {
		newItem, exists := newMap[id]
		if !exists {
			result.Deletions = append(result.Deletions, Entry{
				ID:   id,
				Name: stringField(oldItem, nameField),
			})
			continue
		}
		// Field-level changes.
		for key, newVal := range newItem {
			if ignoreFields[key] {
				continue
			}
			oldVal, exists := oldItem[key]
			if !exists {
				continue
			}
			oldJSON := normalizeJSON(oldVal)
			newJSON := normalizeJSON(newVal)
			if oldJSON != newJSON {
				result.Changes = append(result.Changes, Change{
					ID:     id,
					Field:  key,
					OldVal: oldJSON,
					NewVal: newJSON,
				})
			}
		}
	}

	for id, newItem := range newMap {
		if _, exists := oldMap[id]; !exists {
			result.Additions = append(result.Additions, Entry{
				ID:   id,
				Name: stringField(newItem, nameField),
			})
		}
	}

	slices.SortFunc(result.Additions, func(a, b Entry) int {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	})
	slices.SortFunc(result.Deletions, func(a, b Entry) int {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	})

	return result
}

func stringField(m map[string]any, key string) string {
	if s, ok := m[key].(string); ok {
		return s
	}
	return "<unnamed>"
}

// normalizeJSON converts a value to a canonical JSON string for comparison.
func normalizeJSON(v any) string {
	normalized := normalizeValue(v)
	b, _ := json.Marshal(normalized)
	return string(b)
}

func normalizeValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		out := make(map[string]any, len(val))
		for _, k := range keys {
			out[k] = normalizeValue(val[k])
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = normalizeValue(item)
		}
		return out
	default:
		return v
	}
}
