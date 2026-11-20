package catalog

import (
	"encoding/json"
	"testing"
)

// v0.729.0 dropped hand_craftable from catalog_recipes.json. A recipe is
// hand-craftable when facility_only is false and its category is not
// "Ship Passive"; an explicit hand_craftable (older snapshots) still wins.
func TestRecipeHandCraftableDerived(t *testing.T) {
	cases := []struct {
		name, js string
		want     bool
	}{
		{"new plain", `{"id":"a","category":"Smelting"}`, true},
		{"new facility only", `{"id":"b","category":"Smelting","facility_only":true}`, false},
		{"new ship passive", `{"id":"c","category":"Ship Passive"}`, false},
		{"old explicit false", `{"id":"d","category":"Refining","hand_craftable":false}`, false},
		{"old explicit true", `{"id":"e","category":"Refining","hand_craftable":true,"facility_only":true}`, true},
	}
	for _, c := range cases {
		var r Recipe
		if err := json.Unmarshal([]byte(c.js), &r); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if r.HandCraftable != c.want {
			t.Errorf("%s: HandCraftable = %v, want %v", c.name, r.HandCraftable, c.want)
		}
	}
}
