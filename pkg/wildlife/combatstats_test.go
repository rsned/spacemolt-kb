package wildlife

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadCombatStats(t *testing.T) {
	path := filepath.Join(t.TempDir(), "combat_stats.json")
	body := `{
  "source": "2 exported battle logs",
  "species": {
    "Ash-Scarab": {
      "battles": 2, "hull_min": 45, "hull_max": 45,
      "shield_min": 0, "shield_max": 0,
      "hit_min": 0.08, "hit_max": 0.3,
      "weapons": {
        "Ash-Scarab (natural)": {"damage_type": "kinetic", "base_min": 2, "base_max": 2, "shots": 5}
      }
    },
    "Twin-Type": {
      "battles": 1, "hull_min": 10, "hull_max": 10,
      "shield_min": 0, "shield_max": 0,
      "hit_min": null, "hit_max": null,
      "weapons": {
        "A (natural)": {"damage_type": "void", "base_min": 3, "base_max": 4, "shots": 1},
        "B (natural)": {"damage_type": "energy", "base_min": 1, "base_max": 1, "shots": 2}
      }
    }
  }
}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cs, err := LoadCombatStats(path)
	if err != nil {
		t.Fatalf("LoadCombatStats: %v", err)
	}
	scarab := cs.Species["Ash-Scarab"]
	if scarab.HullMax != 45 || scarab.HitMax != 0.3 || scarab.Battles != 2 {
		t.Errorf("scarab = %+v", scarab)
	}
	if got := scarab.DamageTypes(); !reflect.DeepEqual(got, []string{"kinetic"}) {
		t.Errorf("scarab types = %v", got)
	}
	// null hit ranges parse as zero; multiple weapons yield sorted types.
	twin := cs.Species["Twin-Type"]
	if twin.HitMin != 0 || twin.HitMax != 0 {
		t.Errorf("twin hit range = %v..%v, want 0..0", twin.HitMin, twin.HitMax)
	}
	if got := twin.DamageTypes(); !reflect.DeepEqual(got, []string{"energy", "void"}) {
		t.Errorf("twin types = %v", got)
	}
}
