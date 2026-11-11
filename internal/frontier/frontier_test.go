package frontier

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEstimates(t *testing.T) {
	est := LoadEstimates("../../" + EstimatesPath)
	if e, ok := est["cold_lens"]; !ok || e.X != -5829 || e.Y != 5334 {
		t.Fatalf("cold_lens = %+v (ok=%v)", e, ok)
	}
	if LoadEstimates("/nonexistent.json") != nil {
		t.Error("missing file should give nil")
	}
}

func TestChartedIDs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "get_map.json"), []byte(`{"systems":[{"system_id":"sol"},{"system_id":"arcturus"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	ids := ChartedIDs(dir)
	if !ids["sol"] || !ids["arcturus"] || ids["bristle"] {
		t.Fatalf("ids = %v", ids)
	}
	if ChartedIDs(t.TempDir()) != nil {
		t.Error("no map should give nil")
	}
}
