package render_test

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/render"
)

func TestRenderCloudCubeMapTerranNonNil(t *testing.T) {
	cm := render.RenderCloudCubeMap(planetgen.Profiles["terran"], 42, 64)
	if cm == nil {
		t.Fatal("expected non-nil for terran")
	}
}

func TestRenderCloudCubeMapDisabledForScorched(t *testing.T) {
	cm := render.RenderCloudCubeMap(planetgen.Profiles["scorched"], 42, 64)
	if cm != nil {
		t.Errorf("expected nil for scorched (Cloud.Coverage=0)")
	}
}

func TestRenderCloudCubeMapDisabledForJovian(t *testing.T) {
	cm := render.RenderCloudCubeMap(planetgen.Profiles["jovian"], 42, 64)
	if cm != nil {
		t.Errorf("expected nil for jovian (gas giant — primary render is already cloud-banded)")
	}
}

func TestRenderCloudCubeMapDeterministic(t *testing.T) {
	a := render.RenderCloudCubeMap(planetgen.Profiles["terran"], 42, 64)
	b := render.RenderCloudCubeMap(planetgen.Profiles["terran"], 42, 64)
	if a == nil || b == nil {
		t.Fatal("expected non-nil")
	}
	for f := range a.Faces {
		for i, p := range a.Faces[f] {
			if p != b.Faces[f][i] {
				t.Fatalf("non-deterministic at face %d pixel %d", f, i)
			}
		}
	}
}

func TestRenderRockyDebugRegistersCloudStages(t *testing.T) {
	frame := render.RenderRockyDebug(planetgen.Profiles["terran"], 42, 64, nil)
	var foundAlpha, foundDensity, foundShaded bool
	for _, st := range frame.Stages {
		switch st.Name {
		case "Cloud: alpha":
			foundAlpha = true
		case "Cloud: density":
			foundDensity = true
		case "Cloud: shaded":
			foundShaded = true
		}
	}
	if !foundAlpha {
		t.Errorf("missing Cloud: alpha stage")
	}
	if !foundDensity {
		t.Errorf("missing Cloud: density stage")
	}
	if !foundShaded {
		t.Errorf("missing Cloud: shaded stage")
	}
}

func TestRenderRockyDebugSkipsCloudStagesOnScorched(t *testing.T) {
	frame := render.RenderRockyDebug(planetgen.Profiles["scorched"], 42, 64, nil)
	for _, st := range frame.Stages {
		if st.Name == "Cloud: alpha" || st.Name == "Cloud: density" || st.Name == "Cloud: shaded" {
			t.Errorf("scorched (Coverage=0) should not produce cloud debug stages, got %q", st.Name)
		}
	}
}
