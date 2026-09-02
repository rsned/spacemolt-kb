package main

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/combatsim"
)

func loadForTest(t *testing.T) (*combatsim.Catalog, *combatsim.Calibration) {
	t.Helper()
	cat, err := combatsim.LoadCatalog("../../data/combat-sim/catalog")
	if err != nil {
		t.Fatal(err)
	}
	cal, err := combatsim.LoadCalibration("../../data/combat-sim/calibration.json")
	if err != nil {
		t.Fatal(err)
	}
	return cat, cal
}

func TestBuildMatrixSubset(t *testing.T) {
	cat, cal := loadForTest(t)
	m := BuildMatrix(cat, cal, []string{"prospect", "shard"}, []string{"axiom", "opus_magna"}, 25000, 40, 4000)
	if len(m.Rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(m.Rows))
	}
	// axiom (tier-1 fighter) falls to a small Prospect swarm; opus_magna needs many.
	axiom := m.cell("axiom", "prospect")
	opus := m.cell("opus_magna", "prospect")
	if axiom == 0 || axiom >= opus {
		t.Fatalf("axiom N=%d should be finite and < opus N=%d", axiom, opus)
	}
}

func TestStarterColumns(t *testing.T) {
	cat, _ := loadForTest(t)
	cols, err := starterColumns(cat)
	if err != nil {
		t.Fatal(err)
	}
	ids := starterColumnIDs()
	if len(cols) != len(ids) {
		t.Fatalf("got %d columns, want %d", len(cols), len(ids))
	}
	for i, c := range cols {
		if c.ID != ids[i] {
			t.Errorf("column %d id = %q, want %q", i, c.ID, ids[i])
		}
		if c.Name == "" || c.Empire == "" || c.Weapon == "" || c.DamageType == "" {
			t.Errorf("column %+v has an empty display field", c)
		}
	}
}
