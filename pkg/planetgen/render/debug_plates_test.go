package render_test

import (
	"strings"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/render"
)

func TestRenderRockyDebugIncludesPlateStages(t *testing.T) {
	profile := *planetgen.Profiles["terran"]
	frame := render.RenderRockyDebug(&profile, 17, 32, nil)
	var names []string
	for _, s := range frame.Stages {
		names = append(names, s.Name)
	}
	for _, want := range []string{
		"Plates: id", "Plates: oceanic",
		"Plates: convergent", "Plates: divergent", "Plates: transform",
		"Jitter: cells",
	} {
		var found bool
		for _, n := range names {
			if n == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("stage %q missing from %+v", want, names)
		}
	}
	if frame.Plates == nil {
		t.Error("frame.Plates must be populated when PlateCount > 0")
	}
	if frame.Jitter == nil {
		t.Error("frame.Jitter must be populated when JitterEnabled")
	}
}

func TestRenderRockyDebugZeroPlates(t *testing.T) {
	profile := *planetgen.Profiles["jovian"]
	frame := render.RenderRockyDebug(&profile, 17, 32, nil)
	for _, s := range frame.Stages {
		if strings.HasPrefix(s.Name, "Plates: ") || s.Name == "Jitter: cells" {
			t.Errorf("unexpected stage %q for plate-less profile", s.Name)
		}
	}
	if frame.Plates != nil {
		t.Error("frame.Plates must be nil when PlateCount == 0")
	}
	if frame.Jitter != nil {
		t.Error("frame.Jitter must be nil when JitterEnabled = false")
	}
}
