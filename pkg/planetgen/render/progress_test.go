package render_test

import (
	"bytes"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/render"
)

// TestRenderRockyProgressHookIdentity proves the hook changes no output
// byte: same profile+seed rendered with a nil hook and with a hook set
// must produce identical cube maps, and the hook must actually fire.
func TestRenderRockyProgressHookIdentity(t *testing.T) {
	prof := *planetgen.Profiles["terran"]
	const seed int64 = 42
	const face = 32

	base := render.RenderRocky(&prof, seed, face)

	var stages []string
	render.SetProgressHook(func(stage string, i, n int) { stages = append(stages, stage) })
	t.Cleanup(func() { render.SetProgressHook(nil) })
	hooked := render.RenderRocky(&prof, seed, face)

	if len(stages) == 0 {
		t.Fatal("progress hook never fired")
	}
	var b1, b2 bytes.Buffer
	if err := cubemap.WriteCrossPNGTo(base, &b1); err != nil {
		t.Fatal(err)
	}
	if err := cubemap.WriteCrossPNGTo(hooked, &b2); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b1.Bytes(), b2.Bytes()) {
		t.Fatal("hooked render diverged from nil-hook render")
	}
	want := []string{"render:jitter", "render:plates", "render:heightmap"}
	for i, w := range want {
		if stages[i] != w {
			t.Fatalf("stages[%d] = %q, want %q (full: %v)", i, stages[i], w, stages)
		}
	}
}
