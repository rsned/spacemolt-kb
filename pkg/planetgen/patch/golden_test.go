package patch

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

var updateGoldens = flag.Bool("update", false, "re-bake patch layer goldens")

// TestPatchLayerGoldens is the byte-exact per-layer regression gate:
// it renders every layer of the Patch Lab stack for two crust-enabled
// archetypes and compares a StateHash fingerprint against
// testdata/goldens.json. Run with -update to re-bake after an
// intentional pipeline change.
func TestPatchLayerGoldens(t *testing.T) {
	path := filepath.Join("testdata", "goldens.json")
	got := map[string]string{}
	for _, arch := range []string{"terran", "arid"} {
		base := planetgen.GetProfile(arch)
		if base.Crust.MajorPlates <= 0 {
			t.Fatalf("%s must be crust-enabled", arch)
		}
		// No built-in profile sets Coastal.Amp, so layer 5 (coastal)
		// would be a no-op identity pass and its golden would be
		// indistinguishable from layer 4 (normalize) — pin a real
		// coastal config on a copy (same literal as stack_test.go's
		// countingStack, which needs the same layer actually enabled
		// to make its invocation-count assertions meaningful) so the
		// coastal golden has real coverage.
		prof := *base
		prof.Coastal = types.CoastalConfig{Amp: 0.05, Threshold: 0.5, Freq: 8}
		master := seed.Hash("PatchGolden")
		sd, err := ComputeSphere(&prof, master, 64)
		if err != nil {
			t.Fatal(err)
		}
		w := Pick(sd, 128, 256, 1)[0].Window
		f, err := ExtractFields(sd, w)
		if err != nil {
			t.Fatal(err)
		}
		s := NewStack(&Context{Sphere: sd, Fields: f, Profile: &prof, Master: master})
		for i, l := range Layers() {
			st, err := s.RenderTo(i)
			if err != nil {
				t.Fatal(err)
			}
			got[fmt.Sprintf("%s/%d-%s", arch, i, l.ID)] = StateHash(st)
		}
	}
	if *updateGoldens {
		data, err := json.MarshalIndent(got, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no goldens baked yet — run with -update: %v", err)
	}
	want := map[string]string{}
	if err := json.Unmarshal(data, &want); err != nil {
		t.Fatal(err)
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("layer snapshot drifted: %s got %s want %s", k, got[k], w)
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			t.Errorf("new un-baked layer snapshot: %s (run -update)", k)
		}
	}
}
