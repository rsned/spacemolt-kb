package profilejson_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/profilejson"
)

// TestProfilesDoNotDrift walks every committed envelope under
// data/planet-profiles/ and, for each non-hand-tuned file, verifies it
// matches a freshly-encoded envelope built from GetProfile(env.Type)
// with the same seed and HandTuned=false. Hand-tuned files are
// skipped — the maintainer has opted out of drift checking by
// setting handTuned: true.
//
// Failure means an in-code default changed in a way that the on-disk
// envelope no longer reflects. Fix by either:
//   - rerunning `seed-planet-profiles -force` for the affected planets, or
//   - marking the envelope `"handTuned": true` if it is now intentionally divergent.
//
// Runs in <100ms (pure encode + memcmp, no PNG bake).
func TestProfilesDoNotDrift(t *testing.T) {
	// Discover the repo root by walking up until we find go.mod.
	root := repoRoot(t)
	dir := filepath.Join(root, "data", "planet-profiles")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("no %s directory yet", dir)
		}
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		t.Run(e.Name(), func(t *testing.T) {
			committed, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			env, err := profilejson.Decode(committed)
			if err != nil {
				t.Fatalf("decode %s: %v", e.Name(), err)
			}
			if env.HandTuned {
				t.Skipf("hand-tuned, drift check skipped")
			}
			fresh := &profilejson.Envelope{
				SchemaVersion: profilejson.CurrentSchemaVersion,
				Type:          env.Type,
				Seed:          env.Seed,
				HandTuned:     false,
				Profile:       planetgen.GetProfile(env.Type),
			}
			want, err := profilejson.Encode(fresh)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(committed, want) {
				t.Errorf("envelope %s drifted from GetProfile(%q). "+
					"Either rerun `go run ./cmd/tools/seed-planet-profiles -force -planet=%s:%s` "+
					"or mark this file `\"handTuned\": true`.",
					e.Name(), env.Type, env.Type, env.Seed)
			}
		})
	}
}

// repoRoot walks up from the test working directory until it finds
// go.mod. Tests run from the package directory, so we need to escape
// pkg/planetgen/profilejson/ to find data/planet-profiles/.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for d := wd; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
		if d == filepath.Dir(d) {
			t.Fatalf("go.mod not found from %s", wd)
		}
	}
}
